package stream

import (
	"context"
	"fmt"
	"m3u-stream-merger/logger"
	"m3u-stream-merger/proxy"
	"m3u-stream-merger/proxy/loadbalancer"
	"sync/atomic"
	"time"
)

type StreamHandler struct {
	config      *StreamConfig
	logger      logger.Logger
	coordinator *StreamCoordinator
}

func NewStreamHandler(config *StreamConfig, coordinator *StreamCoordinator, logger logger.Logger) *StreamHandler {
	return &StreamHandler{
		config:      config,
		logger:      logger,
		coordinator: coordinator,
	}
}

type StreamResult struct {
	BytesWritten int64
	Error        error
	Status       int
}

func (h *StreamHandler) HandleStream(
	ctx context.Context,
	lbResult *loadbalancer.LoadBalancerResult,
	writer ResponseWriter,
	remoteAddr string,
) StreamResult {
	if h.coordinator == nil {
		h.logger.Error("handleBufferedStream: coordinator is nil")
		return StreamResult{0, fmt.Errorf("coordinator is nil"), proxy.StatusServerError}
	}

	// Lock the initialization (writer-start) section.
	h.coordinator.initializationMu.Lock()
	// Check if we have already started the writer.
	if !h.coordinator.writerStarted {
		// Mark the writer as started.
		h.coordinator.writerStarted = true

		h.coordinator.writerCtxMu.Lock()
		if h.coordinator.writerCtx == nil {
			h.coordinator.writerCtx, h.coordinator.writerCancel =
				context.WithCancel(context.Background())
		}
		h.coordinator.writerCtxMu.Unlock()

		h.coordinator.lastError.Store((*ChunkData)(nil))
		h.coordinator.clearBuffer()

		// Start the writer in its own goroutine.
		go func() {
			// When the writer stops, reset the flag.
			defer func() {
				h.coordinator.initializationMu.Lock()
				h.coordinator.writerStarted = false
				h.coordinator.initializationMu.Unlock()
			}()
			h.coordinator.StartWriter(h.coordinator.writerCtx, lbResult)
		}()
	}

	if err := h.coordinator.RegisterClient(); err != nil {
		h.coordinator.initializationMu.Unlock()
		return StreamResult{0, err, proxy.StatusServerError}
	}
	h.coordinator.initializationMu.Unlock()

	h.logger.Debugf("Client registered: %s, count: %d", remoteAddr, atomic.LoadInt32(&h.coordinator.clientCount))

	cleanup := func() {
		h.coordinator.UnregisterClient()
		currentCount := atomic.LoadInt32(&h.coordinator.clientCount)
		h.logger.Debugf("Client unregistered: %s, remaining: %d", remoteAddr, currentCount)

		if currentCount == 0 {
			h.coordinator.writerCtxMu.Lock()
			if h.coordinator.writerCancel != nil {
				h.logger.Debug("Stopping writer - no clients remaining")
				h.coordinator.writerCancel()
				h.coordinator.writerCancel = nil
			}
			h.coordinator.writerCtx = nil
			h.coordinator.writerCtxMu.Unlock()

			h.coordinator.lastError.Store((*ChunkData)(nil))
			h.coordinator.clearBuffer()

			h.coordinator.mu.Lock()
			h.coordinator.writerChan = make(chan struct{}, 1)
			h.coordinator.mu.Unlock()
		}
	}
	defer cleanup()

	var bytesWritten int64
	lastPosition := h.coordinator.buffer.Prev() // Start from previous to get first new chunk

	// Create a channel to signal client helper goroutine to stop
	done := make(chan struct{})
	defer close(done)

	// Create a context for this client
	readerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle context cancellation in a separate goroutine
	go func() {
		select {
		case <-ctx.Done():
			h.logger.Debugf("Client context cancelled: %s", remoteAddr)
			cancel()
		case <-done:
			return
		}
	}()

	for {
		select {
		case <-readerCtx.Done():
			h.logger.Debugf("Reader context cancelled for client: %s", remoteAddr)
			return StreamResult{bytesWritten, readerCtx.Err(), proxy.StatusClientClosed}

		default:
			chunks, errChunk, newPos := h.coordinator.ReadChunks(lastPosition)

			// Process any available chunks first
			if len(chunks) > 0 {
				for _, chunk := range chunks {
					// Check context before each write
					if readerCtx.Err() != nil {
						// Clean up remaining chunks
						for _, c := range chunks {
							if c != nil {
								c.Reset()
							}
						}
						return StreamResult{bytesWritten, readerCtx.Err(), proxy.StatusClientClosed}
					}

					if chunk != nil && chunk.Buffer != nil && chunk.Buffer.Len() > 0 {
						// Protect against nil writer
						if writer == nil {
							h.logger.Error("Writer is nil")
							return StreamResult{bytesWritten, fmt.Errorf("writer is nil"), proxy.StatusServerError}
						}

						// Use a separate function for writing to handle panics
						n, err := h.safeWrite(writer, chunk.Buffer.Bytes())
						if err != nil {
							// Clean up remaining chunks
							for _, c := range chunks {
								if c != nil {
									c.Reset()
								}
							}
							return StreamResult{bytesWritten, err, proxy.StatusClientClosed}
						}
						bytesWritten += int64(n)

						if flusher, ok := writer.(StreamFlusher); ok {
							// Protect against panic in flush
							if err := h.safeFlush(flusher); err != nil {
								return StreamResult{bytesWritten, err, proxy.StatusClientClosed}
							}
						}
					}
					if chunk != nil {
						chunk.Reset()
					}
				}
			}

			// Handle any error chunk
			if errChunk != nil {
				if flusher, ok := writer.(StreamFlusher); ok {
					h.safeFlush(flusher)
				}
				return StreamResult{bytesWritten, errChunk.Error, errChunk.Status}
			}

			// Update position if we have a valid new position
			if newPos != nil {
				lastPosition = newPos
			}

			// Small sleep to prevent tight loop when no data
			if len(chunks) == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

// safeWrite attempts to write to the writer and recovers from panics
func (h *StreamHandler) safeWrite(writer ResponseWriter, data []byte) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Errorf("Panic in write: %v", r)
			err = fmt.Errorf("write failed: %v", r)
		}
	}()

	return writer.Write(data)
}

// safeFlush attempts to flush the writer and recovers from panics
func (h *StreamHandler) safeFlush(flusher StreamFlusher) error {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Errorf("Panic in flush: %v", r)
		}
	}()

	flusher.Flush()
	return nil
}

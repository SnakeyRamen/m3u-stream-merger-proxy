package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func GetEnv(env string) string {
	switch env {
	case "USER_AGENT":
		// Set the custom User-Agent header
		userAgent, userAgentExists := os.LookupEnv("USER_AGENT")
		if !userAgentExists {
			userAgent = "IPTV Smarters/1.0.3 (iPad; iOS 16.6.1; Scale/2.00)"
		}
		return userAgent
	default:
		return ""
	}
}

func GetM3UIndexes() []int {
	m3uIndexes := []int{}
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(pair[0], "M3U_URL_") {
			indexString := strings.TrimPrefix(pair[0], "M3U_URL_")
			index, err := strconv.Atoi(indexString)
			if err != nil {
				continue
			}
			m3uIndexes = append(m3uIndexes, index-1)
		}
	}
	return m3uIndexes
}

func GetFilters(baseEnv string) []string {
	filters := []string{}
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if strings.HasPrefix(pair[0], baseEnv) {
			indexString := strings.TrimPrefix(pair[0], fmt.Sprint("%s_", baseEnv))
			_, err := strconv.Atoi(indexString)
			if err != nil {
				continue
			}
			filters = append(filters, pair[1])
		}
	}
	return filters
}

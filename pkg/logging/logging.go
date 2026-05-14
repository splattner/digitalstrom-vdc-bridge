package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Fields map[string]any

var (
	structuredEnabled = detectStructuredEnabled()
	debugEnabled      = detectDebugEnabled()
)

func Info(msg string, fields Fields) {
	emit("info", msg, fields)
}

func Warn(msg string, fields Fields) {
	emit("warn", msg, fields)
}

func Error(msg string, fields Fields) {
	emit("error", msg, fields)
}

func Debug(msg string, fields Fields) {
	if !debugEnabled {
		return
	}
	emit("debug", msg, fields)
}

func IsDebugEnabled() bool {
	return debugEnabled
}

func IsStructuredEnabled() bool {
	return structuredEnabled
}

func emit(level, msg string, fields Fields) {
	if fields == nil {
		fields = Fields{}
	}

	if structuredEnabled {
		record := map[string]any{
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"level": level,
			"msg":   msg,
		}
		for k, v := range fields {
			record[k] = v
		}
		b, err := json.Marshal(record)
		if err != nil {
			log.Printf("LOGGING_MARSHAL_ERROR level=%s msg=%s err=%v", level, msg, err)
			return
		}
		log.Print(string(b))
		return
	}

	if len(fields) == 0 {
		log.Printf("%s %s", strings.ToUpper(level), msg)
		return
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, fields[k]))
	}
	log.Printf("%s %s %s", strings.ToUpper(level), msg, strings.Join(parts, " "))
}

func detectStructuredEnabled() bool {
	if parseEnvBool("VDCGO_LOG_JSON") {
		return true
	}
	fmtEnv := strings.TrimSpace(strings.ToLower(os.Getenv("VDCGO_LOG_FORMAT")))
	return fmtEnv == "json"
}

func detectDebugEnabled() bool {
	if parseEnvBool("VDCGO_LOG_DEBUG") {
		return true
	}
	return parseEnvBool("VDCGO_DEBUG")
}

func parseEnvBool(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

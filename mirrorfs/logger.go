package mirrorfs

import "github.com/sirupsen/logrus"

func loggerWith(fields map[string]interface{}) *logrus.Entry {
	log := logrus.New()
	log.SetLevel(getLoggingLevel())

	return log.WithFields(logrus.Fields(fields))
}

var (
	levelMapping = map[string]logrus.Level{
		"debug": logrus.DebugLevel,
		"info":  logrus.InfoLevel,
		"warn":  logrus.WarnLevel,
		"error": logrus.ErrorLevel,
	}
)

func SetLogLevel(level string) {
	loggingLevel = level
}

var loggingLevel string

func getLoggingLevel() logrus.Level {
	val, exists := levelMapping[loggingLevel]
	if !exists {
		return logrus.WarnLevel
	}
	return val
}

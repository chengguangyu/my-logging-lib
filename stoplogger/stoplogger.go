package stoplogger

import . "github.com/comodo/comodo-logging-lib/startlogger"

func StopLogger(LogServer Logger) {
	LogServer.ShutDown()
}

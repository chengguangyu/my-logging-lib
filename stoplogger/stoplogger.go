package stoplogger

import . "github.com/comodo/comodoca-logging-lib/startlogger"

func StopLogger(LogServer Logger) {
	LogServer.ShutDown()
}

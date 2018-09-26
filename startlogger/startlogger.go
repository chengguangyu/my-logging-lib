package startlogger

import (
	"bytes"
	"fmt"
	"github.com/comodo/comodo-logging-lib/config"
	"github.com/fatih/color"
	"github.com/robfig/cron"
	"gopkg.in/natefinch/lumberjack.v2"
	logger "log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

var printer chan string

func PrintMsg(log LogMessage) {
	var buffer bytes.Buffer

	buffer.WriteString("|")

	var colorPrint *color.Color
	switch {
	case log.Level == "dmp":
		colorPrint = color.New(color.FgHiCyan)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("dmp"))
	case log.Level == "dbg":
		colorPrint = color.New(color.FgHiBlue)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("dbg"))
	case log.Level == "wrn":
		colorPrint = color.New(color.FgYellow)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("wrn"))
	case log.Level == "err":
		colorPrint = color.New(color.FgRed)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("pnc"))
	case log.Level == "logDone":
		colorPrint = color.New(color.FgGreen)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("fin"))
	case log.Level == "logFail":
		colorPrint = color.New(color.FgRed)
		g := colorPrint.SprintfFunc()
		buffer.WriteString(g("err"))
	default:
		buffer.WriteString("fyi")
	}

	buffer.WriteString("|")

	var host string

	log.Host = strings.ToLower(log.Host)

	switch {
	case strings.HasSuffix(log.Host, "api-services"):
		host = "api"
	case strings.HasSuffix(log.Host, "registration-service"):
		host = "reg"
	case strings.HasSuffix(log.Host, "web-app"):
		host = "app"
	case strings.HasSuffix(log.Host, "web-ng"):
		host = "wng"
	case strings.HasSuffix(log.Host, "cert-service"):
		host = "cer"
	case strings.HasSuffix(log.Host, "download-service"):
		host = "dow"
	case strings.HasSuffix(log.Host, "scheduler-service"):
		host = "sch"
	case strings.HasSuffix(log.Host, "certinfo-service"):
		host = "cei"
	case strings.HasSuffix(log.Host, "notification-service"):
		host = "not"
	case strings.HasSuffix(log.Host, "signer-services"):
		host = "sig"
	default:
		host = fmt.Sprintf("%-30s", log.Host)
	}

	splitHost := strings.Split(log.Host, "-")
	//Hack to check for host length until we make clients send better names
	if len(host) == 3 && len(splitHost) > 0 {
		host = host + "|" + splitHost[len(splitHost)-1] //Take the last item since it will be the containerID
	}
	buffer.WriteString(host)

	buffer.WriteString("|")

	if len(log.Id) > 0 {
		buffer.WriteString(log.Id)
	} else {
		buffer.WriteString("■■■■■■■■■■■■■■■■■■■■■■■■")
	}

	// TODO: I don't think we need to log this.
	tm := strconv.FormatInt(log.Ts, 10)
	buffer.WriteString("|")
	buffer.WriteString(tm)

	buffer.WriteString("|")

	if log.Msg != nil {
		temp := log.Msg.(interface{})
		var message bytes.Buffer

		switch vv := temp.(type) {
		case string:
			message.WriteString(vv)
		case []string:
			for i, v := range vv {
				if i != 0 {
					message.WriteString("\n")
				}
				message.WriteString(v)
			}
		}

		switch {
		case log.Level == "api":
			colorPrint = color.New(color.FgHiCyan)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "cer":
			colorPrint = color.New(color.FgHiBlue)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "wrn":
			colorPrint = color.New(color.FgYellow)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "err":
			colorPrint = color.New(color.FgRed)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "logDone":
			colorPrint = color.New(color.FgGreen)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "logFail":
			colorPrint = color.New(color.FgRed)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		default:
			buffer.WriteString(message.String())
		}
	}

	//printer <- fmt.Sprint(buffer.String())
	logger.Println(buffer.String())
}

func createPrinter() {
	printer = make(chan string, 1)
	go func() {
		for {
			msg := <-printer
			fmt.Println(msg)
		}
	}()
}

func WriteLog() {

	createPrinter()

	// create lumberjack logger  https://github.com/natefinch/lumberjack
	l := &lumberjack.Logger{
		Filename:   config.Config.LogPath + "/comodo.log",
		MaxSize:    config.Config.MaxSize, // megabytes
		MaxBackups: config.Config.MaxBackups,
		MaxAge:     config.Config.MaxAge, //days
	}

	logger.SetFlags(logger.LUTC | logger.Ldate | logger.Ltime)

	logger.SetOutput(l)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for {
			<-c
			fmt.Println("Resetting Logging file.")
			l.Rotate()
		}
	}()

	cr := cron.New()
	cr.AddFunc("@midnight", func() {
		fmt.Println("Resetting Logging file.")
		l.Rotate()
	})
	cr.Start()

	runtime.Goexit()
}

func StartLogServer(serverName string, hostName string, logServer Logger) {

	config.Config.Load("conf.json")
	logServer.Connect(config.Config.RabbitMQUrl, serverName, hostName, true)
	logServer.CreateQueue(logServer.GetLoggerChannel())
	go func() {
		logServer.ParseLog(logServer.GetLoggerChannel(), logServer.GetLoggerQueue().Name)
	}()

}

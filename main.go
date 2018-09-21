package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/comodo/comodo-logging-lib/config"
	"github.com/fatih/color"
	"github.com/robfig/cron"
	"github.com/streadway/amqp"
	"gopkg.in/natefinch/lumberjack.v2"
	logger "log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

type LogMessage struct {
	Id    string
	Host  string
	Msg   interface{}
	Ts    int64
	Level string
	Data  string
}

var rabbitCh *amqp.Channel

var printer chan string

func failOnError(err error, msg string) {
	if err != nil {
		logger.Fatalf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}

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

	// TODO: This could be done on the clients.
	log.Host = strings.ToLower(log.Host)

	switch {
	case strings.HasPrefix(log.Host, "hworker-q"):
		host = "que"
	case strings.HasPrefix(log.Host, "hworker-t"):
		host = "trg"
	case strings.HasPrefix(log.Host, "hserver"):
		host = "srv"
	case strings.HasPrefix(log.Host, "streams"):
		host = "str"
	case strings.HasPrefix(log.Host, "fraudserver"):
		host = "frd"
	case strings.HasPrefix(log.Host, "gatekeeper"):
		host = "gkp"
	case strings.HasPrefix(log.Host, "socket-server"):
		host = "soc"
	case strings.HasPrefix(log.Host, "push-server"):
		host = "pus"
	case strings.HasPrefix(log.Host, "hushed-backoffice-server"):
		host = "sup"
	case strings.HasPrefix(log.Host, "maelstrom-collector_test"):
		host = "mae"
	case strings.HasPrefix(log.Host, "maelstrom-uploader"):
		host = "mau"
	case strings.HasPrefix(log.Host, "hushed-store"):
		host = "sto"
	default:
		host = fmt.Sprintf("%-30s", log.Host)
	}

	splitHost := strings.Split(log.Host, "-")
	//Hack to check for host length until we make clients send better names
	if len(host) == 3 && len(splitHost) > 0 {
		host = host + "|" + splitHost[len(splitHost)-1] //Take the last item since it will be the containerID
	}

	// TODO Proper colorization. But it should be based on log level not host.
	//if host[0] == 'h' {
	//	colorPrint = color.New(color.FgMagenta)
	//	colorPrint.Add(color.Faint)
	//	g := colorPrint.SprintfFunc()
	//	buffer.WriteString(g(host))
	//} else if host[0] == 's' {
	//	colorPrint = color.New(color.FgCyan)
	//	colorPrint.Add(color.Faint)
	//	g := colorPrint.SprintfFunc()
	//	buffer.WriteString(g(host))
	//} else {
	buffer.WriteString(host)
	//}

	buffer.WriteString("|")

	if len(log.Id) > 0 {
		buffer.WriteString(log.Id)
	} else {
		buffer.WriteString("■■■■■■■■■■■■■■■■■■■■■■■■")
	}

	// TODO: I don't think we need to log this.
	// tm := strconv.FormatInt(log.Ts, 10)
	// buffer.WriteString("|")
	// buffer.WriteString(tm)

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
		case log.Level == "dmp":
			colorPrint = color.New(color.FgHiCyan)
			g := colorPrint.SprintfFunc()
			buffer.WriteString(g(message.String()))
		case log.Level == "dbg":
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

func ParseMsg(m *amqp.Publishing) {
	message := m.Body
	log := LogMessage{}

	if err := json.Unmarshal(message, &log); err != nil {
		fmt.Printf("ERROR PANIC: %s\n", err)
		return
	}

	PrintMsg(log)
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

func RabbitMQConnect() {
	var url string
	var err error

	url = Config.RabbitMQUrl

	if url == "" {
		panic("No connection url set")
	}

	fmt.Println("Connecting to " + url)

	conn, err := amqp.Dial(url)
	failOnError(err, "Failed to connect to RabbitMQ")
	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()
	defer conn.Close()
	//TODO: Setup callbacks to be notified on disconnects, reconnects and connection closed.

}

func main() {
	config.Config.Load("conf.json")

	createPrinter()

	// create lumberjack logger  https://github.com/natefinch/lumberjack
	l := &lumberjack.Logger{
		Filename:   config.Config.LogPath + "/hushed.log",
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

	RabbitMQConnect()

	runtime.Goexit()
}

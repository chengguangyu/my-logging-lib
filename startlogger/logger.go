package startlogger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/streadway/amqp"
	"log"
	"runtime"
	"time"
)

const GOOD = "good"
const DEBUG = "dbg"
const WARNING = "wrn"
const ERROR = "err"
const FYI = "fyi"
const PANIC = "panic"

type LogMessage struct {
	Id    string
	Host  string
	Msg   interface{}
	Ts    int64
	Level string
}

var server string
var host string
var logChannel *amqp.Channel

type LoggerInterface interface {
	Connect(RabbitMQUrl string, serverName string, hostName string, fatal bool)
	CreateQueue(ch *amqp.Channel) bool
	CreateTopicExchange(ch *amqp.Channel) bool
	BindQueuesToExchange(ch *amqp.Channel, qName string, keys []string)
	GetLoggerChannel() *amqp.Channel
	GetLoggerQueue() amqp.Queue
	ShutDown()
	ParseLog(ch *amqp.Channel, qName string)
	PrintLocally(printLocal bool)
	Warn(v ...interface{})
	Warnf(format string, v ...interface{})
	WarnfId(id, format string, v ...interface{})
	Error(err error, v ...interface{})
	Errorf(err error, format string, v ...interface{})
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	PrintfId(id, format string, v ...interface{})
	PrintLevel(level string, v ...interface{})
	PrintfLevel(level string, format string, v ...interface{})
	Panic(v ...interface{})
	Panicf(format string, v ...interface{})
	publishLog(text string, level string)
	publishLogId(text string, level string, id string)
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}

type Logger struct {
	LoggerInterface
	rabbitCh     *amqp.Channel
	rabbitConn   *amqp.Connection
	rabbitQueue  amqp.Queue
	printLocally bool
}

func (logger *Logger) PrintLocally(printLocal bool) {
	logger.printLocally = printLocal
}

func (logger *Logger) Connect(RabbitMQUrl string, serverName string, hostName string, fatal bool) {
	conn, err := amqp.Dial(RabbitMQUrl)

	failOnError(err, "Failed to connect to RabbitMQ")
	logger.rabbitConn = conn

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")

	logger.rabbitCh = ch
	logChannel = ch
	server = serverName
	host = hostName
}

func (logger *Logger) ShutDown() {
	logger.rabbitConn.Close()
	logger.rabbitCh.Close()
	fmt.Println("logger closed")
}

func (logger *Logger) GetLoggerChannel() *amqp.Channel {
	return logger.rabbitCh
}

func (logger *Logger) GetLoggerQueue() amqp.Queue {
	return logger.rabbitQueue
}

func (logger *Logger) CreateQueue(ch *amqp.Channel) bool {

	q, err := ch.QueueDeclare(
		"",    // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")
	fmt.Print("logger started")
	logger.rabbitQueue = q
	return true
}

func (logger *Logger) CreateTopicExchange(ch *amqp.Channel) bool {

	err := ch.ExchangeDeclare(
		"logs",  // name
		"topic", // type
		true,    // durable
		false,   // auto-deleted
		false,   // internal
		false,   // no-wait
		nil,     // arguments
	)
	failOnError(err, "Failed to declare a topic")
	return true
}
func (logger *Logger) BindQueuesToExchange(ch *amqp.Channel, qName string, keys []string) {
	for _, routingKey := range keys {
		err := ch.QueueBind(
			qName,
			routingKey,
			"logs",
			false,
			nil)
		failOnError(err, "Failed to bind a queue")
	}
}

func (logger *Logger) publishLog(text string, level string) {
	now := time.Now()
	var milli = now.UnixNano() / 1000000
	message := LogMessage{Level: level, Host: server + "-" + host, Msg: text, Ts: milli}

	b, err := json.Marshal(message)
	if err != nil {
		fmt.Println("error:", err)
	}

	err = logChannel.Publish(
		"logs",               // exchange
		GetRoutingKey(level), // routing key
		false,                // mandatory
		false,                // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(b),
		})
	failOnError(err, "Failed to publish a message")
}

func (logger *Logger) ParseLog(ch *amqp.Channel, qName string) {

	msgs, err := ch.Consume(
		qName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Failed to register a consumer")

	forever := make(chan bool)

	go func() {
		for msg := range msgs {
			logMsg := LogMessage{}
			if err := json.Unmarshal(msg.Body, &logMsg); err != nil {
				fmt.Printf("Cannot parse the log message: %s\n", err)
				return
			}
			msg.Ack(false)
			PrintMsg(logMsg)
		}
	}()
	<-forever
}

func (logger *Logger) publishLogId(text string, level string, id string) {
	now := time.Now()
	var milli = now.UnixNano() / 1000000
	message := LogMessage{Level: level, Host: server + "-" + host, Msg: text, Ts: milli, Id: id}
	b, err := json.Marshal(message)
	if err != nil {
		fmt.Println("error:", err)
	}
	err = logChannel.Publish(
		"logs",               // exchange
		GetRoutingKey(level), // routing key
		false,                // mandatory
		false,                // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(b),
		})
	failOnError(err, "Failed to publish a message")
}

func printLog(text string, level string) {
	now := time.Now()
	var milli = now.UnixNano() / 1000000
	formattedText := fmt.Sprintf("[%s] %d - %s", host, milli, text)

	switch level {
	case GOOD:
		CommandGood(formattedText)
	case DEBUG:
		CommandDebug(formattedText)
	case WARNING:
		CommandWarn(formattedText)
	case ERROR:
		CommandFail(formattedText)
	case FYI:
		CommandFyi(formattedText)
	case PANIC:
		CommandPanic(formattedText)
	}
}

func handleError() string {

	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("Stacktrace:\n"))
	i := 2
	for i < 40 {
		if function1, file1, line1, ok := runtime.Caller(i); ok {
			buffer.WriteString(fmt.Sprintf("      at %s (%s:%d)\n", runtime.FuncForPC(function1).Name(), file1, line1))
		} else {
			break
		}
		i++
	}

	return buffer.String()
}

func (logger *Logger) Warn(v ...interface{}) {
	logger.publishLog(fmt.Sprint(v...), WARNING)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), WARNING)
	}
}

func (logger *Logger) Warnf(format string, v ...interface{}) {
	logger.publishLog(fmt.Sprintf(format, v...), WARNING)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), WARNING)
	}
}

func (logger *Logger) Error(err error, v ...interface{}) {
	msg := fmt.Sprint(v...)
	if msg == "" {
		msg = err.Error()
	}
	if err != nil {
		msg = msg + "\n" + handleError()
	}
	logger.publishLog(msg, ERROR)
	printLog(msg, ERROR)
}

func (logger *Logger) Errorf(err error, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)

	if err != nil {
		msg = msg + "\n" + err.Error() + "\n" + handleError()
	}
	logger.publishLog(msg, ERROR)
	printLog(msg, ERROR)
}

func (logger *Logger) Fatal(v ...interface{}) {
	logger.publishLog(fmt.Sprint(v...), ERROR)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), PANIC)
	}
	log.Panic(v...)
}

func (logger *Logger) Fatalf(format string, v ...interface{}) {
	logger.publishLog(fmt.Sprintf(format, v...), ERROR)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), PANIC)
	}
	log.Panicf(format, v...)
}
func (logger *Logger) Print(v ...interface{}) {
	logger.publishLog(fmt.Sprint(v...), FYI)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), FYI)
	}
}

func (logger *Logger) Printf(format string, v ...interface{}) {
	logger.publishLog(fmt.Sprintf(format, v...), FYI)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), FYI)
	}
}

func (logger *Logger) PrintId(id string, v ...interface{}) {
	logger.publishLogId(fmt.Sprint(v...), FYI, id)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), FYI)
	}
}

func (logger *Logger) PrintfId(id string, format string, v ...interface{}) {
	logger.publishLogId(fmt.Sprintf(format, v...), FYI, id)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), FYI)
	}
}

func (logger *Logger) PrintLevel(level string, v ...interface{}) {
	logger.publishLog(fmt.Sprint(v...), level)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), level)
	}
}

func (logger *Logger) PrintfLevel(level string, format string, v ...interface{}) {
	logger.publishLog(fmt.Sprintf(format, v...), level)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), level)
	}
}

func (logger *Logger) Panic(v ...interface{}) {
	logger.publishLog(fmt.Sprint(v...), ERROR)
	if logger.printLocally {
		printLog(fmt.Sprint(v...), PANIC)
	}
	log.Panic(v...)
}

func (logger *Logger) Panicf(format string, v ...interface{}) {
	logger.publishLog(fmt.Sprintf(format, v...), ERROR)
	if logger.printLocally {
		printLog(fmt.Sprintf(format, v...), PANIC)
	}
	log.Panicf(format, v...)
}

package startlogger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/streadway/amqp"
	"log"
	"runtime"
	"sync"
	"time"
)

const GOOD = "good"
const DEBUG = "dbg"
const WARNING = "wrn"
const ERROR = "err"
const FYI = "fyi"
const PANIC = "panic"

var routingKeys map[string]string

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
	CreateQueue(ch *amqp.Channel) amqp.Queue
	CreateTopicExchange(ch *amqp.Channel) bool
	CreateConsumer(ch *amqp.Channel, q amqp.Queue) <-chan amqp.Delivery
	BindQueueToExchange(ch *amqp.Channel, q amqp.Queue, routingKey string)
	GetLoggerChannel() *amqp.Channel
	ShutDown()
	StartReceiver(ch *amqp.Channel) []amqp.Delivery
	ConsumeMsgs(msgs []amqp.Delivery)
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
func (logger *Logger) CreateQueue(ch *amqp.Channel, qName string) amqp.Queue {
	q, err := ch.QueueDeclare(
		qName, // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	failOnError(err, "Failed to declare a queue")
	return q
}

func (logger *Logger) BindQueueToExchange(ch *amqp.Channel, q amqp.Queue, routingKey string) {
	err := ch.QueueBind(
		q.Name,
		routingKey,
		"logs",
		false,
		nil)
	failOnError(err, "Failed to bind a queue")
}

func (logger *Logger) CreateConsumer(ch *amqp.Channel, q amqp.Queue) <-chan amqp.Delivery {
	msgs, err := ch.Consume(
		q.Name,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Failed to register a consumer")
	return msgs
}

func (logger *Logger) publishLog(text string, level string) {
	now := time.Now()
	var milli = now.UnixNano() / 1000000
	message := LogMessage{Level: level, Host: server + "-" + host, Msg: text, Ts: milli}

	b, err := json.Marshal(message)
	if err != nil {
		fmt.Println("error:", err)
	}
	key := GetRoutingKey(level, routingKeys)
	fmt.Print(key)

	err = logChannel.Publish(
		"logs",                            // exchange
		GetRoutingKey(level, routingKeys), // routing key
		false,                             // mandatory
		false,                             // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(b),
		})
	failOnError(err, "Failed to publish a message")
}

func (logger *Logger) StartReceiver(ch *amqp.Channel) []<-chan amqp.Delivery {

	routingKeys = LoadRoutingKeys()
	delivers := make([]<-chan amqp.Delivery, 1)

	for level, routingKey := range routingKeys {
		q := logger.CreateQueue(ch, level)
		logger.BindQueueToExchange(ch, q, routingKey)
		msgs := logger.CreateConsumer(ch, q)
		fmt.Print(msgs)
		delivers = append(delivers, msgs)
	}
	return delivers
}
func (logger *Logger) ConsumeMsgs(delivers []<-chan amqp.Delivery) {
	forever := make(chan int, 50)
	var wg sync.WaitGroup
	wg.Add(len(delivers))
	for _, msgs := range delivers {

		go func(msgs <-chan amqp.Delivery) {
			for msg := range msgs {
				_, ok := <-forever
				if !ok {
					wg.Done()
					return
				}
				logMsg := LogMessage{}

				if err := json.Unmarshal(msg.Body, &logMsg); err != nil {
					fmt.Printf("Cannot parse the log message: %s\n", err)
					return
				}
				msg.Ack(false)
				PrintMsg(logMsg)

			}
			log.Println("waiting for logs")
		}(msgs)

	}
	for i := 0; i < 50; i++ {
		forever <- i
	}
	close(forever)
	wg.Wait()
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
		"logs",                            // exchange
		GetRoutingKey(level, routingKeys), // routing key
		false,                             // mandatory
		false,                             // immediate
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

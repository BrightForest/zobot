package main

import (
	"crypto/tls"
	"encoding/json"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

var (
	Trace   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func LogInit(
	traceHandle io.Writer,
	infoHandle io.Writer,
	warningHandle io.Writer,
	errorHandle io.Writer) {

	Trace = log.New(traceHandle,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoHandle,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(warningHandle,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

type ImagesSender struct {
	Bot                *tgbotapi.BotAPI
	ImagesFromThreads  map[string]string
	ImagesSendedToChat map[string]string
	FileSenderChannel  chan *FileMessage
	ThreadsNumberServe map[string]bool
	CheckRate          int
	TelegramBotToken   string
	TelegramGroupId    int64
	FindRegexes        []string
}

type FileMessage struct {
	Link   string
	Thread string
}

type ThreadData struct {
	Threads []struct {
		Posts []struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"posts"`
	} `json:"threads"`
}

type ThreadsList struct {
	Board   string `json:"board"`
	Threads []struct {
		Num     int    `json:"num"`
		Subject string `json:"subject"`
	} `json:"threads"`
}

func (imageSender *ImagesSender) GetThreadsList() {
	tr := &http.Transport{
		IdleConnTimeout:     1000 * time.Millisecond * time.Duration(5),
		TLSHandshakeTimeout: 1000 * time.Millisecond * time.Duration(5),
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	getThreadsRequest, _ := http.NewRequest(
		"GET",
		"https://2ch.hk/b/threads.json",
		nil,
	)
	resp, err := client.Do(getThreadsRequest)
	if err != nil {
		Error.Println(err)
		return
	}
	defer resp.Body.Close()
	decode := json.NewDecoder(resp.Body)
	var list ThreadsList

	errors := decode.Decode(&list)
	if errors != nil {
		Error.Println(err)
		return
	}
	var rSlice []*regexp.Regexp
	for _, r := range imageSender.FindRegexes {
		rx := regexp.MustCompile(r)
		rSlice = append(rSlice, rx)
	}
	var lastThreads []string
	for _, thread := range list.Threads {
		finded := false
		for _, r := range rSlice {
			if r.MatchString(thread.Subject) {
				finded = true
				break
			}
		}
		if finded {
			imageSender.ThreadsNumberServe[strconv.Itoa(thread.Num)] = true
			imageSender.GetPicturesListFromThread(strconv.Itoa(thread.Num))
			lastThreads = append(lastThreads, strconv.Itoa(thread.Num))
		}
	}
	for _, threadNum := range lastThreads {
		if _, ok := imageSender.ThreadsNumberServe[threadNum]; !ok {
			delete(imageSender.ThreadsNumberServe, threadNum)
			for _, thread := range imageSender.ImagesFromThreads {
				delete(imageSender.ImagesFromThreads, thread)
			}
			for _, thread := range imageSender.ImagesSendedToChat {
				delete(imageSender.ImagesSendedToChat, thread)
			}
		}
	}
}

func (imageSender *ImagesSender) GetPicturesListFromThread(threadNumber string) {
	tr := &http.Transport{
		IdleConnTimeout:     1000 * time.Millisecond * time.Duration(10),
		TLSHandshakeTimeout: 1000 * time.Millisecond * time.Duration(10),
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	getThreadsRequest, _ := http.NewRequest(
		"GET",
		"https://2ch.hk/b/res/"+threadNumber+".json",
		nil,
	)
	resp, err := client.Do(getThreadsRequest)
	if err != nil {
		Error.Println(err)
		return
	}
	defer resp.Body.Close()
	decode := json.NewDecoder(resp.Body)
	var tData ThreadData
	errors := decode.Decode(&tData)
	if errors != nil {
		Error.Println(err)
		return
	}
	for _, file := range tData.Threads {
		for _, post := range file.Posts {
			for _, file := range post.Files {
				link := "https://2ch.hk" + file.Path
				imageSender.ImagesFromThreads[link] = threadNumber
			}
		}
	}
	imageSender.NewImagesSender()
}

func (imageSender *ImagesSender) NewImagesSender() {
	for link, thread := range imageSender.ImagesFromThreads {
		if _, ok := imageSender.ImagesSendedToChat[link]; !ok {
			imageSender.FileSenderChannel <- &FileMessage{
				link,
				thread,
			}
			imageSender.ImagesSendedToChat[link] = thread
		}
	}
}

func (imageSender *ImagesSender) ParseScheduler() {
	for {
		imageSender.GetThreadsList()
		time.Sleep(time.Duration(imageSender.CheckRate) * time.Second)
	}
}

func (imageSender *ImagesSender) SendScheduler() {
	for {
		select {
		case filemessage := <-imageSender.FileSenderChannel:
			time.Sleep(3 * time.Second)
			imageSender.SendToChat(filemessage.Link)
		}
	}
}

func (imageSender *ImagesSender) SendToChat(message string) {
	time.Sleep(1 * time.Second)
	msg := tgbotapi.NewMessage(imageSender.TelegramGroupId, message)
	imageSender.Bot.Send(msg)
}

func getBot(token string) *tgbotapi.BotAPI {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		Error.Println("Bot cannot loaded:", err)
		return nil
	} else {
		return bot
	}
}

func getCheckerRateFromEnv() int {
	getRate := os.Getenv("CHECK_RATE_SECONDS")
	if getRate == "" {
		return 60
	} else {
		checkrate, err := strconv.Atoi(getRate)
		if err != nil {
			return 60
		}
		return checkrate
	}
}

func getTelegramBotToken() string {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		Error.Println("Unable to get telegram bot token.")
		return ""
	} else {
		return token
	}
}

func getTelegramGroupId() int64 {
	groupId := os.Getenv("GROUP_ID")
	if groupId == "" {
		Error.Println("Unable to get group id")
		return 0
	} else {
		getIntId, err := strconv.ParseInt(groupId, 10, 64)
		if err != nil {
			Error.Println("Unable to get group id")
			return 0
		} else {
			return getIntId
		}
	}
}

func (sender *ImagesSender) SelfTestEnvVariables() {
	if sender.TelegramBotToken == "" {
		Error.Println("Telegram bot token is not defined.")
		os.Exit(1)
	}
	if sender.TelegramGroupId == 0 {
		Error.Println("Telegram group ID is not defined.")
		os.Exit(1)
	}
	Info.Println("Environment variables ready.")
}

func OSListener(osChannel chan os.Signal) {
	sig := <-osChannel
	Info.Printf("Bot stopped: %+v", sig)
	os.Exit(0)
}

func GetRegexes() []string {
	return []string{
		".*ЗАСМЕ.*",
		".*ОБОСРА.*",
		".*зАсМе.*",
		"(?i).*засме.*",
		".*ЗАСМІЯВ.*",
	}
}

func init() {
	LogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	GetRegexes()
	sender := ImagesSender{
		nil,
		make(map[string]string),
		make(map[string]string),
		make(chan *FileMessage),
		make(map[string]bool),
		getCheckerRateFromEnv(),
		getTelegramBotToken(),
		getTelegramGroupId(),
		GetRegexes(),
	}
	sender.SelfTestEnvVariables()
	sender.Bot = getBot(sender.TelegramBotToken)
	go sender.SendScheduler()
	go sender.ParseScheduler()
	Info.Println("Bot started.")
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	OSListener(gracefulStop)
}

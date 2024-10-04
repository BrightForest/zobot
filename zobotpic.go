package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	_ "github.com/lib/pq"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	GET_SUBSCRIBERS = "SELECT \"chatid\", \"username\", \"isactive\" FROM public.\"subscribers\""
	ADD_SUBSCRIBER  = "INSERT INTO public.\"subscribers\" (chatid, username, isactive) VALUES ($1, $2, $3)"
	CHANGE_STATE    = "UPDATE public.\"subscribers\" SET isactive = $2 WHERE \"chatid\" = $1"
	GET_REGEXES     = "SELECT \"reg\" FROM public.\"regexes\";"
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

type Bot struct {
	is          *ImagesSender
	subscribers map[int64]*Subscriber
	pgConn      string
	db          *sql.DB
}

type ImagesSender struct {
	Bot                *tgbotapi.BotAPI
	ImagesFromThreads  map[string]string
	ImagesSendedToChat map[string]string
	FileSenderChannel  chan *FileMessage
	ThreadsNumberServe map[string]bool
	CheckRate          int
	TelegramBotToken   string
	FindRegexes        []string
}

type Subscriber struct {
	ChatId        int64
	Username      string
	IsActive      bool
	HasBlockedBot bool
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
		Error.Println(errors)
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
			Info.Println("Thread", thread.Num, "added to threads list.")
			lastThreads = append(lastThreads, strconv.Itoa(thread.Num))
		}
	}
	for _, threadNum := range lastThreads {
		if _, ok := imageSender.ThreadsNumberServe[threadNum]; !ok {
			delete(imageSender.ThreadsNumberServe, threadNum)
			Info.Println("Thread", threadNum, "was deleted from threads list.")
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
		Error.Println(errors)
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

func (b *Bot) SendScheduler() {
	for {
		select {
		case filemessage := <-b.is.FileSenderChannel:
			for _, s := range b.subscribers {
				if s.IsActive && !s.HasBlockedBot {
					b.is.SendToChat(s, filemessage.Link, b)
					time.Sleep(3 * time.Second)
				}
			}
		}
	}
}

func (imageSender *ImagesSender) SendToChat(s *Subscriber, message string, b *Bot) {
	msg := tgbotapi.NewMessage(s.ChatId, message)
	_, err := imageSender.Bot.Send(msg)
	if err != nil {
		imageSender.SendErrorProcessing(err, s, b)
	}
}

func (imageSender *ImagesSender) SendErrorProcessing(sendErr error, s *Subscriber, b *Bot) {
	st := regexp.MustCompile(".*Forbidden.*")
	if st.MatchString(sendErr.Error()) {
		b.subscribers[s.ChatId].HasBlockedBot = true
		Info.Println(s.Username, "with chatId", s.ChatId, "has disabled bot.")
		return
	}
	Error.Println(sendErr.Error())
}

func getBot(token string) *tgbotapi.BotAPI {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		Error.Println("Bot cannot loaded:", err)
		return nil
	} else {
		Info.Println("Bot authorized with account:", bot.Self.UserName)
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

func CreatePGConnString() string {
	pgconn := make(map[string]string)
	pgconn["POSTGRES_USER"] = os.Getenv("POSTGRES_USER")
	pgconn["POSTGRES_PASSWORD"] = os.Getenv("POSTGRES_PASSWORD")
	pgconn["POSTGRES_HOST"] = os.Getenv("POSTGRES_HOST")
	pgconn["POSTGRES_PORT"] = os.Getenv("POSTGRES_PORT")
	pgconn["POSTGRES_DB"] = os.Getenv("POSTGRES_DB")
	for k, v := range pgconn {
		if v == "" {
			Error.Println("Env variable for database is empty:", k)
			os.Exit(1)
		}
	}
	b := strings.Builder{}
	b.WriteString("postgresql://")
	b.WriteString(pgconn["POSTGRES_USER"])
	b.WriteString(":")
	b.WriteString(pgconn["POSTGRES_PASSWORD"])
	b.WriteString("@")
	b.WriteString(pgconn["POSTGRES_HOST"])
	b.WriteString(":")
	b.WriteString(pgconn["POSTGRES_PORT"])
	b.WriteString("/")
	b.WriteString(pgconn["POSTGRES_DB"])
	b.WriteString("?sslmode=disable")
	return b.String()
}

func (b *Bot) GetDBConn() {
	db, err := sql.Open("postgres", b.pgConn)
	if err != nil {
		Error.Println(err)
		os.Exit(1)
	}
	b.db = db
}

func (b *Bot) GetSubsFromDB() {
	stmt, err := b.db.Prepare(GET_SUBSCRIBERS)
	if err != nil {
		Error.Println(err)
		os.Exit(1)
	}
	rows, err1 := stmt.Query()
	if err1 != nil {
		Error.Println(err1)
		os.Exit(1)
	}
	defer rows.Close()
	defer stmt.Close()
	for rows.Next() {
		var (
			chatid   int64
			username string
			isactive bool
		)
		err := rows.Scan(
			&chatid,
			&username,
			&isactive,
		)
		if err != nil {
			Error.Println(err)
		}
		s := Subscriber{
			ChatId:        chatid,
			Username:      username,
			IsActive:      isactive,
			HasBlockedBot: false,
		}
		b.subscribers[s.ChatId] = &s
	}
	Info.Println("Subscribers loaded count:", len(b.subscribers))
}

func (b *Bot) AddSubsToDB(s *Subscriber) {
	stmt, err := b.db.Prepare(ADD_SUBSCRIBER)
	if err != nil {
		Error.Println(err)
	}
	defer stmt.Close()
	if _, err1 := stmt.Exec(
		s.ChatId,
		s.Username,
		s.IsActive,
	); err1 != nil {
		Error.Println(err1)
	}
}

func (b *Bot) ChangeSubsState(chatid int64, requestedState bool) {
	b.subscribers[chatid].IsActive = requestedState
	stmt, err := b.db.Prepare(CHANGE_STATE)
	if err != nil {
		Error.Println(err)
	}
	defer stmt.Close()
	if _, err1 := stmt.Exec(
		chatid,
		requestedState,
	); err1 != nil {
		Error.Println(err1)
	}
}

func (b *Bot) GetRegexesFromDB() {
	stmt, err := b.db.Prepare(GET_REGEXES)
	if err != nil {
		Error.Println(err)
		os.Exit(1)
	}
	rows, err1 := stmt.Query()
	if err1 != nil {
		Error.Println(err1)
		os.Exit(1)
	}
	defer rows.Close()
	defer stmt.Close()
	ra := []string{}
	for rows.Next() {
		var (
			reg string
		)
		err := rows.Scan(
			&reg,
		)
		if err != nil {
			Error.Println(err)
		}
		ra = append(ra, reg)
	}
	if len(ra) < 1 {
		Error.Println("Regexes table in DB is empty.")
		os.Exit(1)
	}
	b.is.FindRegexes = ra
}

func (b *Bot) UpdateRegexesScheduler() {
	for {
		time.Sleep(60 * time.Second)
		b.GetRegexesFromDB()
	}
}

func (sender *ImagesSender) SelfTestEnvVariables() {
	if sender.TelegramBotToken == "" {
		Error.Println("Telegram bot token is not defined.")
		os.Exit(1)
	}
	Info.Println("Environment variables ready.")
}

func OSListener(osChannel chan os.Signal) {
	sig := <-osChannel
	Info.Printf("Bot stopped: %+v", sig)
	os.Exit(0)
}

func Load() {
	sender := ImagesSender{
		nil,
		make(map[string]string),
		make(map[string]string),
		make(chan *FileMessage),
		make(map[string]bool),
		getCheckerRateFromEnv(),
		getTelegramBotToken(),
		nil,
	}
	sender.SelfTestEnvVariables()
	sender.Bot = getBot(sender.TelegramBotToken)
	b := Bot{
		is:          &sender,
		subscribers: make(map[int64]*Subscriber),
		pgConn:      CreatePGConnString(),
		db:          nil,
	}
	b.GetDBConn()
	b.GetRegexesFromDB()
	b.GetSubsFromDB()
	go b.SendScheduler()
	go b.is.ParseScheduler()
	go b.UpdateRegexesScheduler()
	go b.UpdatesReciever()
	Info.Println("Bot started.")
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	OSListener(gracefulStop)
}

func (b *Bot) UpdatesReciever() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := b.is.Bot.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil { // ignore any non-Message updates
			continue
		}

		if !update.Message.IsCommand() { // ignore any non-command Messages
			continue
		}

		if _, ok := b.subscribers[update.Message.Chat.ID]; !ok {
			s := Subscriber{
				ChatId:   update.Message.Chat.ID,
				Username: update.Message.Chat.UserName,
				IsActive: false,
			}
			b.subscribers[update.Message.Chat.ID] = &s
			b.AddSubsToDB(&s)
			Info.Println("New subscriber added with username:", update.Message.Chat.UserName)
		}

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

		switch update.Message.Command() {
		case "start":
			msg.Text = "Жмём /disco чтобы начать залив мемасов."
		case "help":
			msg.Text = "Можно прислать команду /pause, чтобы остановить поток шитшторма.\nКоманда /disco возобновит залив мемасов."
		case "pause":
			msg.Text = "Мемсный шторм остановлен."
			b.ChangeSubsState(update.Message.Chat.ID, false)
			Info.Println(update.Message.Chat.UserName, "paused bot.")
		case "disco":
			msg.Text = "Мемсный шторм включён."
			b.ChangeSubsState(update.Message.Chat.ID, true)
			Info.Println(update.Message.Chat.UserName, "activated bot.")
		default:
			msg.Text = "Неизвестная команда."
		}
		if _, err := b.is.Bot.Send(msg); err != nil {
			Error.Println(err)
		}
	}
}

func init() {
	LogInit(io.Discard, os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	Load()
}

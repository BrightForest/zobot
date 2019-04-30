package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)

type ImagesSender struct {
	Bot *tgbotapi.BotAPI
	ImagesFromThreads map[string]string
	ImagesSendedToChat map[string]string
	FileSenderChannel chan *FileMessage
	ThreadsNumberServe map[string]bool
	CheckRate int
	TelegramBotToken string
	TelegramGroupId int64
}

type FileMessage struct {
	Link string
	Thread string
}

type ThreadData struct {
	Board             string `json:"Board"`
	BoardInfo         string `json:"BoardInfo"`
	BoardInfoOuter    string `json:"BoardInfoOuter"`
	BoardName         string `json:"BoardName"`
	AdvertBottomImage string `json:"advert_bottom_image"`
	AdvertBottomLink  string `json:"advert_bottom_link"`
	AdvertMobileImage string `json:"advert_mobile_image"`
	AdvertMobileLink  string `json:"advert_mobile_link"`
	AdvertTopImage    string `json:"advert_top_image"`
	AdvertTopLink     string `json:"advert_top_link"`
	BoardBannerImage  string `json:"board_banner_image"`
	BoardBannerLink   string `json:"board_banner_link"`
	BumpLimit         int    `json:"bump_limit"`
	CurrentThread     string `json:"current_thread"`
	DefaultName       string `json:"default_name"`
	EnableDices       int    `json:"enable_dices"`
	EnableFlags       int    `json:"enable_flags"`
	EnableIcons       int    `json:"enable_icons"`
	EnableImages      int    `json:"enable_images"`
	EnableLikes       int    `json:"enable_likes"`
	EnableNames       int    `json:"enable_names"`
	EnableOekaki      int    `json:"enable_oekaki"`
	EnablePosting     int    `json:"enable_posting"`
	EnableSage        int    `json:"enable_sage"`
	EnableShield      int    `json:"enable_shield"`
	EnableSubject     int    `json:"enable_subject"`
	EnableThreadTags  int    `json:"enable_thread_tags"`
	EnableTrips       int    `json:"enable_trips"`
	EnableVideo       int    `json:"enable_video"`
	FilesCount        int    `json:"files_count"`
	IsBoard           int    `json:"is_board"`
	IsClosed          int    `json:"is_closed"`
	IsIndex           int    `json:"is_index"`
	MaxComment        int    `json:"max_comment"`
	MaxFilesSize      int    `json:"max_files_size"`
	MaxNum            int    `json:"max_num"`
	NewsAbu           []struct {
		Date    string `json:"date"`
		Num     int    `json:"num"`
		Subject string `json:"subject"`
		Views   int    `json:"views"`
	} `json:"news_abu"`
	PostsCount int `json:"posts_count"`
	Threads    []struct {
		Posts []struct {
			Banned  int    `json:"banned"`
			Closed  int    `json:"closed"`
			Comment string `json:"comment"`
			Date    string `json:"date"`
			Email   string `json:"email"`
			Endless int    `json:"endless"`
			Files   []struct {
				Displayname string `json:"displayname"`
				Fullname    string `json:"fullname"`
				Height      int    `json:"height"`
				Md5         string `json:"md5"`
				Name        string `json:"name"`
				Nsfw        int    `json:"nsfw"`
				Path        string `json:"path"`
				Size        int    `json:"size"`
				Thumbnail   string `json:"thumbnail"`
				TnHeight    int    `json:"tn_height"`
				TnWidth     int    `json:"tn_width"`
				Type        int    `json:"type"`
				Width       int    `json:"width"`
			} `json:"files"`
			Lasthit   int    `json:"lasthit"`
			Name      string `json:"name"`
			Num       int    `json:"num"`
			Number    int    `json:"number"`
			Op        int    `json:"op"`
			Parent    string `json:"parent"`
			Sticky    int    `json:"sticky"`
			Subject   string `json:"subject"`
			Tags      string `json:"tags,omitempty"`
			Timestamp int    `json:"timestamp"`
			Trip      string `json:"trip"`
		} `json:"posts"`
	} `json:"threads"`
	Title string `json:"title"`
	Top   []struct {
		Board string `json:"board"`
		Info  string `json:"info"`
		Name  string `json:"name"`
	} `json:"top"`
	UniquePosters string `json:"unique_posters"`
}

type ThreadsList struct {
	Board   string `json:"board"`
	Threads []struct {
		Comment    string  `json:"comment"`
		Lasthit    int     `json:"lasthit"`
		Num        string  `json:"num"`
		PostsCount int     `json:"posts_count"`
		Score      float64 `json:"score"`
		Subject    string  `json:"subject"`
		Timestamp  int     `json:"timestamp"`
		Views      int     `json:"views"`
	} `json:"threads"`
}

func (imageSender *ImagesSender) GetThreadsList() {
	tr := &http.Transport{
		IdleConnTimeout: 1000 * time.Millisecond * time.Duration(5),
		TLSHandshakeTimeout: 1000 * time.Millisecond * time.Duration(5),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport:tr}
	getThreadsRequest, _ := http.NewRequest(
		"GET",
		"https://2ch.hk/b/threads.json",
		nil,
	)
	resp, err := client.Do(getThreadsRequest)
	if err != nil{
		Error.Println(err)
		return
	}
	defer resp.Body.Close()
	decode := json.NewDecoder(resp.Body)
	var list ThreadsList
	errors := decode.Decode(&list)
	if errors != nil{
		Error.Println(err)
		return
	}
	regex1 := regexp.MustCompile(".*ЗАСМЕ.*")
	regex2 := regexp.MustCompile(".*ОБОСРА.*")
	var lastThreads []string
	for _, thread := range list.Threads{
		matches1 := regex1.MatchString(thread.Subject)
		matches2 := regex2.MatchString(thread.Subject)
		if matches1 || matches2{
			imageSender.ThreadsNumberServe[thread.Num] = true
			imageSender.GetPicturesListFromThread(thread.Num)
			lastThreads = append(lastThreads, thread.Num)
		}
	}
	for _, threadNum := range lastThreads{
		if _, ok := imageSender.ThreadsNumberServe[threadNum]; !ok{
			delete(imageSender.ThreadsNumberServe, threadNum)
			for _, thread := range imageSender.ImagesFromThreads{
				delete(imageSender.ImagesFromThreads, thread)
			}
			for _, thread := range imageSender.ImagesSendedToChat{
				delete(imageSender.ImagesSendedToChat, thread)
			}
		}
	}
}

func (imageSender *ImagesSender) GetPicturesListFromThread(threadNumber string){
	tr := &http.Transport{
		IdleConnTimeout: 1000 * time.Millisecond * time.Duration(5),
		TLSHandshakeTimeout: 1000 * time.Millisecond * time.Duration(5),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport:tr}
	getThreadsRequest, _ := http.NewRequest(
		"GET",
		"https://2ch.hk/b/res/" + threadNumber + ".json",
		nil,
	)
	resp, err := client.Do(getThreadsRequest)
	if err != nil{
		Error.Println(err)
		return
	}
	defer resp.Body.Close()
	decode := json.NewDecoder(resp.Body)
	var tData ThreadData
	errors := decode.Decode(&tData)
	if errors != nil{
		Error.Println(err)
		return
	}
	for _, file := range tData.Threads{
		for _, post := range file.Posts{
			for _, file := range post.Files{
				link := "https://2ch.hk" + file.Path
				imageSender.ImagesFromThreads[link] = threadNumber
			}
		}
	}
	imageSender.NewImagesSender()
}

func (imageSender *ImagesSender) NewImagesSender(){
	for link, thread := range imageSender.ImagesFromThreads{
		if _, ok := imageSender.ImagesSendedToChat[link]; !ok {
			imageSender.FileSenderChannel <- &FileMessage{
				link,
				thread,
			}
			imageSender.ImagesSendedToChat[link] = thread
		}
	}
}

func (imageSender *ImagesSender) ParseScheduler(){
	for{
		imageSender.GetThreadsList()
		time.Sleep(30*time.Second)
	}
}

func (imageSender *ImagesSender) SendScheduler(){
	for{
		select {
		case filemessage := <- imageSender.FileSenderChannel:
			time.Sleep(3*time.Second)
			imageSender.SendToChat(filemessage.Link)
		}
	}
}

func (imageSender *ImagesSender) SendToChat(message string){
	time.Sleep(1*time.Second)
	msg := tgbotapi.NewMessage(-1001456741130, message)
	imageSender.Bot.Send(msg)
}

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

func getBot(token string) *tgbotapi.BotAPI{
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil{
		Error.Println("Bot cannot loaded.")
		return nil
	} else {
		return bot
	}
}

func getCheckerRateFromEnv() int{
	getRate := os.Getenv("CHECK_RATE_SECONDS")
	if getRate == ""{
		return 60
	} else {
		checkrate, err := strconv.Atoi(getRate)
		if err != nil{
			return 60
		}
		return checkrate
	}
}

func getTelegramBotToken() string{
	token := os.Getenv("BOT_TOKEN")
	if token == ""{
		Error.Println("Unable to get telegram bot token.")
		return ""
	} else {
		return token
	}
}

func getTelegramGroupId() int64{
	groupId := os.Getenv("GROUP_ID")
	if groupId == ""{
		Error.Println("Unable to get group id")
		return 0
	} else {
		getIntId, err := strconv.ParseInt(groupId, 10, 64)
		if err != nil{
			Error.Println("Unable to get group id")
			return 0
		} else {
			return getIntId
		}
	}
}

func init() {
	LogInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	sender := ImagesSender{
		nil,
		make(map[string]string),
		make(map[string]string),
		make(chan *FileMessage),
		make(map[string]bool),
		getCheckerRateFromEnv(),
		getTelegramBotToken(),
		getTelegramGroupId(),
	}
	sender.Bot = getBot(sender.TelegramBotToken)
	go sender.SendScheduler()
	go sender.ParseScheduler()
	Info.Println("Bot started.")
	for{
		time.Sleep(2 * time.Second)
	}
}

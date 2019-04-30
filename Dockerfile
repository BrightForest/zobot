FROM golang:1.11.2
RUN mkdir /bot
WORKDIR /bot
ADD . /bot/
RUN go get github.com/go-telegram-bot-api/telegram-bot-api
RUN go build -o zobotpic .
RUN groupadd -r zobotpic && useradd -r -g zobotpic zobotpic
USER zobotpic
CMD ["/bot/zobotpic"]
# Засмеялся-обосрался бот

Нужен для того, чтобы искать в /b/ треды "засмеялся-обосрался" и репостить оттуда линки на картинки в телеграм канал.

## Переменные среды для бота

```
# Как часто бот чекает появление новых тредов и картинок
CHECK_RATE_SECONDS=60
# Токен для телеграм бота. Зарегистрировать бота и получить для него токен можно у @BotFather
BOT_TOKEN=""
# ID телеграм группы, куда бот будет постить картинки. Подсмотреть можно, создав группу, добавив в неё бота, где-то в веб-интерфейсе телеграма. И добавить "-100" перед ID.
GROUP_ID=""
```

## Сборка бота для запуска как бинарь

```
git clone https://github.com/BrightForest/zobot.git
cd zobot
go mod download
go build .
cp zobot /bin
```

Образец systemd юнита для запуска бота:

```
[Unit]
Description=Zasmeyalsya-Obosralsya Bot
After=network.target

[Service]
Type=simple
Restart=no
RestartSec=1
User=zobot
Group=zobot
ExecStart=/bin/zobot
Environment="CHECK_RATE_SECONDS=60"
Environment="BOT_TOKEN=XXXX"
Environment="GROUP_ID=-100123456789"

[Install]
WantedBy=multi-user.target
```

## Запуск бота в Docker

```
git clone https://github.com/BrightForest/zobot.git
cd zobot
docker build -t zobot:latest .
docker run -d -e BOT_TOKEN=MYTOKEN -e GROUP_ID=MYGROUID zobot:latest
```
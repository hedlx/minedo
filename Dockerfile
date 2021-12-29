FROM golang:alpine

WORKDIR /usr/src/bot
COPY . .

RUN go build

WORKDIR /srv/minedo
RUN cp /usr/src/bot/minedo .
RUN rm -rf /usr/src/bot

ENTRYPOINT ./minedo -bot

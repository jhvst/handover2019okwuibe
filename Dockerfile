FROM alpine:3.9.2

RUN apk add ffmpeg
ADD handover2019okwuibe /bin/handover2019okwuibe
CMD /bin/handover2019okwuibe | ffmpeg -i - -vf format=gray -c:v libvpx -b:v 1M -f rtp udp://0.0.0.0:1234

EXPOSE 8080

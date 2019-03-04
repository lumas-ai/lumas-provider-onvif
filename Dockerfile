FROM golang:alpine as build

ENV FFMPEG_VERSION=4.1 BUILD_PREFIX=/opt/ffmpeg

RUN apk add --update build-base curl nasm tar bzip2 \
  zlib-dev openssl-dev yasm-dev lame-dev libogg-dev \
  x264-dev libvpx-dev libvorbis-dev x265-dev  \
  freetype-dev libass-dev libwebp-dev libtheora-dev \
  opus-dev && \
  DIR=$(mktemp -d) && cd ${DIR} && \
  curl -s http://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.gz | tar zxvf - -C . && \
  cd ffmpeg-${FFMPEG_VERSION} && \
  ./configure \
  --enable-shared --enable-version3 --enable-gpl --enable-nonfree --enable-small --enable-libmp3lame --enable-libx264 --enable-libx265 --enable-libvpx --enable-libtheora --enable-libvorbis --enable-libopus --enable-libass --enable-libwebp --enable-postproc --enable-avresample --enable-libfreetype --enable-openssl --disable-debug --prefix=${BUILD_PREFIX} && \
  make && \
  make install && \
  rm -rf ${DIR} && \
  apk del build-base curl tar bzip2 x264 openssl nasm && rm -rf /var/cache/apk/* && \
  ln -s /opt/ffmpeg/bin/ffmpeg /usr/bin/ffmpeg

ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib
ENV PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig

RUN apk add --update git pkgconfig gcc libc-dev \
    openssl lame libogg libvpx libvorbis libass \
    freetype libtheora opus libwebp x264 x264-libs x265 && \
    go get -u github.com/3d0c/gmf && \
    go get -u github.com/golang/protobuf/proto && \
    go get -u github.com/golang/protobuf/ptypes/struct && \
    go get -u google.golang.org/grpc && \
    go get -u github.com/lumas-ai/lumas-provider-onvif && \
    cd / && go build /go/src/github.com/lumas-ai/lumas-provider-onvif/cmd/onvif/onvif-server.go



FROM alpine:3.9

ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib
ENV PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig

COPY --from=build /opt/ffmpeg /opt/ffmpeg
COPY --from=build /onvif-server /onvif-server

RUN apk add --update openssl lame libogg libvpx libvorbis libass \
    freetype libtheora opus libwebp x264 x264-libs x265

ENTRYPOINT ["/onvif-server"]

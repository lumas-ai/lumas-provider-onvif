FROM alpine:3.9

ENV LD_LIBRARY_PATH=/opt/ffmpeg/lib
ENV PKG_CONFIG_PATH=/opt/ffmpeg/lib/pkgconfig

COPY --from=lumas/lumas-provider-onvif-build-image /opt/ffmpeg /opt/ffmpeg
COPY --from=lumas/lumas-provider-onvif-build-image /onvif-server /onvif-server

RUN apk add --update openssl lame libogg libvpx libvorbis libass \
    freetype libtheora opus libwebp x264 x264-libs x265

ENTRYPOINT ["/onvif-server"]

#/usr/bin/env bash
docker build . -t lumas/lumas-provider-onvif-build-image

if [ $? -eq 0 ]; then
  echo ""
  echo "Done building lumas/lumas-provider-onvif-build-image"
  echo 'Push to Docker Hub with `docker push lumas/lumas-provider-onvif-build-image`'
fi

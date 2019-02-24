package main

import (
  "fmt"
  "flag"
  "log"
  "net"
  "context"
  "time"
  "google.golang.org/grpc"

  . "github.com/3d0c/gmf"
  api "github.com/lumas-ai/lumas-core/protos/golang/provider"
)

var (
	tls        = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile   = flag.String("cert_file", "", "The TLS cert file")
	keyFile    = flag.String("key_file", "", "The TLS key file")
	iface      = flag.String("host", "0.0.0.0", "The interface to listen on")
	port       = flag.Int("port", 5390, "The server port")
)

type CameraServer struct { }

func (s *CameraServer) StreamRTP(config *api.RTPConfig, stream api.Camera_StreamRTPServer) error {
  var videoOutputSDP string
  var audioOutputSDP string
  var audioOutputCtx *FmtCtx
  var videoOutputCtx *FmtCtx
  var sentFrames int
  var skippedFrames int

  //Create the input context that will stream the RTSP feed from the camera
  rs := config.CameraConfig.Config.GetFields()["rtspStream"].GetStringValue()
  inputCtx, err := NewInputCtx(rs)
  if err != nil {
    log.Println(err)
    return err
  }
  defer inputCtx.Free()
  inputCtx.Dump()

  //Set up video output context
  vist, err := inputCtx.GetBestStream(AVMEDIA_TYPE_VIDEO)
  if err != nil {
    log.Println("The camera does not support video")
  } else {
    videoRTPString := fmt.Sprintf("rtp://%s:%d", config.RtpAddress, config.VideoRTPPort)
    videoOutputCtx, err = NewOutputCtxWithFormatName(videoRTPString, "rtp")
    if err != nil {
      log.Println("Could not create output context: " + err.Error())
      return err
    }
    defer videoOutputCtx.Free()
    videoOutputCtx.SetStartTime(0)
    if _, err = videoOutputCtx.AddStreamWithCodeCtx(vist.CodecCtx()); err != nil {
      log.Println(err.Error())
    }
    videoOutputCtx.Dump()
    videoOutputSDP = videoOutputCtx.GetSDPString()
  }

  //Set up audio output context
  aist, err := inputCtx.GetBestStream(AVMEDIA_TYPE_AUDIO)
  if err != nil {
    log.Println("The camera does not support audio")
  } else {
    audioRTPString := fmt.Sprintf("rtp://%s:%d", config.RtpAddress, config.AudioRTPPort)
    audioOutputCtx, err = NewOutputCtxWithFormatName(audioRTPString, "rtp")
    if err != nil {
      log.Println("Could not create output context: " + err.Error())
      return err
    }
    defer audioOutputCtx.Free()
    audioOutputCtx.SetStartTime(0)
    if _, err = audioOutputCtx.AddStreamWithCodeCtx(aist.CodecCtx()); err != nil {
      log.Println(err.Error())
    }
    audioOutputCtx.Dump()
    audioOutputSDP = audioOutputCtx.GetSDPString()
  }

  if err = videoOutputCtx.WriteHeader(); err != nil {
    log.Fatal(err)
  }

  if err = audioOutputCtx.WriteHeader(); err != nil {
    log.Fatal(err)
  }

  go func() {
    for {
      packet, err := inputCtx.GetNextPacket()
      if err != nil {
        packet.Free()
        skippedFrames++
        continue
      }

      if packet.StreamIndex() != vist.Index() {
        //It's an audio packet

        if aist == nil {
          //This should never happen
          log.Println("Could not read from audio input stream despite receiving an audio packet")
          packet.Free()
          continue
        }

        //The packet's stream index needs to match the stream index (0) of the RTP stream
        packet.SetStreamIndex(0)

        err = audioOutputCtx.WritePacket(packet)
        if err != nil {
          log.Println("Could not write audio packet" + err.Error())
          packet.Free()
          skippedFrames++
          continue
        } else {
          sentFrames++
        }
      } else {
        //The packet's stream index needs to match the stream index (0) of the RTP stream
        packet.SetStreamIndex(0)
        err = videoOutputCtx.WritePacket(packet)
        if err != nil {
          log.Println("Could not write video packet" + err.Error())
          packet.Free()
          skippedFrames++
          continue
        } else {
          sentFrames++
        }
      }

      packet.Free()
    }
  }()

  //Send the first response with the SDP information
  sdp := api.SDP{
    Audio: audioOutputSDP,
    Video: videoOutputSDP,
  }
  r := api.StreamInfo{
    Sdp: &sdp,
  }
  stream.Send(&r)

  //Send a status update every few seconds
  for {
    r := api.StreamInfo{
      SentFrames: int64(sentFrames),
      DroppedFrames: int64(skippedFrames),
    }
    if err := stream.Send(&r); err != nil {
      return err
    }

    time.Sleep(1* time.Second)
  }

  inputCtx.Free()
  return nil
}

func (s *CameraServer) Snapshot(ctx context.Context, config *api.CameraConfig) (*api.Image, error) {
  return new(api.Image), nil
}

func main() {
  flag.Parse()
  lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *iface, *port))
  if err != nil {
    log.Fatalf("failed to listen: %v", err)
  }
  log.Printf("Listening on %s:%d", *iface, *port)

  s := CameraServer{}
  grpcServer := grpc.NewServer()
  api.RegisterCameraServer(grpcServer, &s)
  grpcServer.Serve(lis)
}

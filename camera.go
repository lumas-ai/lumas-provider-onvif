package camera

import (
  "fmt"
  "log"

  . "github.com/3d0c/gmf"
  api "github.com/lumas-ai/lumas-core/protos/golang/provider"
)

type Camera struct {
  open bool
  inputCtx *FmtCtx
  SentFrames int
  DroppedFrames int
}

func (s *Camera) StartRTPStream(config *api.RTPConfig, vsdp chan<- string, asdp chan<- string) error {
  var audioOutputCtx *FmtCtx
  var videoOutputCtx *FmtCtx

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
    vsdp <- videoOutputCtx.GetSDPString()
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
    asdp <- audioOutputCtx.GetSDPString()
  }

  if err = videoOutputCtx.WriteHeader(); err != nil {
    log.Fatal(err)
  }

  if err = audioOutputCtx.WriteHeader(); err != nil {
    log.Fatal(err)
  }

  fmt.Println(s.open)
  s.open = true
  defer s.Close()
  for {
    //If the camera has been closed
    //stop processing packets
    if !s.open {
      break
    }

    packet, err := inputCtx.GetNextPacket()
    if err != nil {
      fmt.Println(err.Error())
      packet.Free()
      s.DroppedFrames++
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
        s.DroppedFrames++
        continue
      } else {
        s.SentFrames++
      }
    } else {
      //The packet's stream index needs to match the stream index (0) of the RTP stream
      packet.SetStreamIndex(0)
      err = videoOutputCtx.WritePacket(packet)
      if err != nil {
        log.Println("Could not write video packet" + err.Error())
        packet.Free()
        s.DroppedFrames++
        continue
      } else {
        s.SentFrames++
      }
    }

    packet.Free()
  }

  return nil
}

func (s *Camera) Close() error {
  if s.open {
    s.open = false
    return nil
  }

  return nil
}

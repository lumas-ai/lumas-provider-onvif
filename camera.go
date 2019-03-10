package camera

import (
  "fmt"
  "log"
  "errors"
  "sync"
  "crypto/md5"
  "encoding/hex"

  . "github.com/3d0c/gmf"
  api "github.com/lumas-ai/lumas-core/protos/golang/provider"
)

type Camera struct {
  Config *api.RTPConfig
  inputCtx *FmtCtx
  SentFrames int
  DroppedFrames int
  closeChan chan bool
  ServerCloseChan chan bool
}

func (s *Camera) getRTSPURL() (string, error) {
  if len(s.Config.CameraConfig.Config.GetFields()) == 0 {
    return "", errors.New("No configuration provided to ONVIF provider")
  }

  return s.Config.CameraConfig.Config.GetFields()["rtspStream"].GetStringValue(), nil
}

func (s *Camera) GenerateCameraID() (string, error) {
  name, err := s.getRTSPURL()
  if err != nil {
    return "", err
  }

  hasher := md5.New()
  hasher.Write([]byte(name))
  return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *Camera) StartRTPStream(vsdp chan<- string, asdp chan<- string) error {
  var audioOutputCtx *FmtCtx
  var videoOutputCtx *FmtCtx

  //Create the input context that will stream the RTSP feed from the camera
  rs, err := s.getRTSPURL()
  if err != nil {
    return err
  }

  inputCtx, err := NewInputCtx(rs)
  if err != nil {
    log.Println(err)
    return err
  }
  defer inputCtx.Free()
  defer inputCtx.CloseInput()
  inputCtx.Dump()

  //Set up video output context
  vist, err := inputCtx.GetBestStream(AVMEDIA_TYPE_VIDEO)
  if err != nil {
    vsdp <- "" //Push an empty string or the application hangs
    log.Println("The camera does not support video")
  } else {
    videoRTPString := fmt.Sprintf("rtp://%s:%d", s.Config.RtpAddress, s.Config.VideoRTPPort)
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

    //Instantiate the the output context and push its SDP to the channel
    //to send to the RTP client at the other end
    videoOutputCtx.Dump()
    vsdp <- videoOutputCtx.GetSDPString()

    if err = videoOutputCtx.WriteHeader(); err != nil {
      log.Fatal(err)
    }
  }

  //Set up audio output context
  aist, err := inputCtx.GetBestStream(AVMEDIA_TYPE_AUDIO)
  if err != nil {
    asdp <- "" //Push an emptry string or the application hangs
    log.Println("The camera does not support audio")
  } else {
    audioRTPString := fmt.Sprintf("rtp://%s:%d", s.Config.RtpAddress, s.Config.AudioRTPPort)
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

    if err = audioOutputCtx.WriteHeader(); err != nil {
      log.Fatal(err)
    }
  }

  defer s.Close()
  //Make the channel that we'll use to monitor for when the stream should end
  //The Camera.Close() method pushes close events to the channel
  s.closeChan = make(chan bool)

  //Get the packets and stream them to the RTP client
  for {
    packet, err := inputCtx.GetNextPacket()
    if err != nil {
      fmt.Println(err.Error())
      packet.Free()
      s.DroppedFrames++
      continue
    }

    //Make sure the camera supports video before we start processing video frames
    //It would very strange if this is nil
    if vist != nil {
      if packet.StreamIndex() == vist.Index() { //Is this a video packet
        //The packet's stream index needs to match the stream index (0) of the RTP stream
        packet.SetStreamIndex(0)
        err = videoOutputCtx.WritePacket(packet)
        if err != nil {
          log.Println("Could not write video packet: " + err.Error())
          packet.Free()
          s.DroppedFrames++
          continue
        } else {
          s.SentFrames++
        }
      } else {
        //It must have been and audio packet
        err := s.processAudioPacket(packet, audioOutputCtx, aist)
        if err != nil {
          packet.Free()
          continue
        }
      }
    } else {
      err := s.processAudioPacket(packet, audioOutputCtx, aist)
      if err != nil {
        packet.Free()
        continue
      }
    }

    packet.Free()

    //If the camera has been closed
    //stop processing packets
    select {
    case _ = <-s.closeChan:
      //Let the close channel know that we're done
      s.closeChan <-true

      inputCtx.CloseInput()
      return nil
    default:
    }
  }
}

func (s *Camera) processAudioPacket(packet *Packet, octx *FmtCtx, aist *Stream) (error) {
  if aist == nil {
    //This should never happen
    errString := "Could not read from audio input stream despite receiving an audio packet"
    log.Println(errString)
    return errors.New(errString)
  }

  //The packet's stream index needs to match the stream index (0) of the RTP stream
  packet.SetStreamIndex(0)

  err := octx.WritePacket(packet)
  if err != nil {
    errString := "Could not write audio packet: " + err.Error()
    log.Println(errString)
    s.DroppedFrames++
    return errors.New(errString)
  } else {
    s.SentFrames++
  }

  return nil
}

func (s *Camera) Close() error {
  var wg sync.WaitGroup
  wg.Add(2)

  go func() {
    defer wg.Done()
    if s.ServerCloseChan != nil {
      s.ServerCloseChan <-true
      _ = <-s.ServerCloseChan
    }
  }()

  go func() {
    defer wg.Done()
    s.closeChan <- true
    _ = <-s.closeChan //Wait to recieve the all clear
  }()

  wg.Wait()
  return nil
}

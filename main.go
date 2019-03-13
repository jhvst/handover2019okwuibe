package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/pions/rtcp"
	"github.com/pions/rtp"
	"github.com/pions/rtp/codecs"
	"github.com/pions/webrtc"
	"github.com/pions/webrtc/pkg/media"
	"github.com/pions/webrtc/pkg/media/ivfwriter"
	"github.com/pions/webrtc/pkg/media/samplebuilder"
)

func (svc *KeyService) saveToDisk(i media.Writer, track *webrtc.Track) {
	defer func() {
		if err := i.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	svc.ready <- true
	for {
		packet, err := track.ReadRTP()
		if err != nil {
			log.Fatal(err)
		}

		if err := i.AddPacket(packet); err != nil {
			log.Fatal(err)
		}
	}
}

// Decode decodes the input from base64
// It can optionally unzip the input after decoding
func Decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}

// Encode encodes the input in base64
// It can optionally zip the input before encoding
func Encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

type KeyService struct {
	recv, sndr chan string
	ready      chan bool
	conn       *webrtc.PeerConnection
}

func (svc *KeyService) handler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	svc.recv <- vars["key"]
	offer := <-svc.sndr
	w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, offer)
}

func (svc *KeyService) demo(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadFile("demo.html")
	if err != nil {
		fmt.Fprintf(w, "%s", err.Error())
		return
	}
	fmt.Fprintf(w, string(body))
}

func main() {

	svc := KeyService{
		recv:  make(chan string),
		sndr:  make(chan string),
		ready: make(chan bool),
	}

	go func(svc KeyService) {
		r := mux.NewRouter()
		r.HandleFunc("/demo", svc.demo)
		r.HandleFunc("/keygen/{key}", svc.handler)
		log.Fatal(http.ListenAndServe(":8080", r))
	}(svc)

	// Create a MediaEngine object to configure the supported codec
	m := webrtc.MediaEngine{}

	// Setup the codecs you want to use.
	// We'll use a VP8 codec but you can also define your own
	m.RegisterCodec(webrtc.NewRTPVP8Codec(webrtc.DefaultPayloadTypeVP8, 90000))

	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		log.Fatal(err)
	}
	svc.conn = peerConnection

	// Set a handler for when a new remote track starts, this handler saves buffers to disk as
	// an ivf file, since we could have multiple video tracks we provide a counter.
	// In your application this is where you would handle/process video
	svc.conn.OnTrack(func(track *webrtc.Track, receiver *webrtc.RTPReceiver) {
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				errSend := svc.conn.SendRTCP(&rtcp.PictureLossIndication{MediaSSRC: track.SSRC()})
				if errSend != nil {
					log.Println(errSend)
				}
			}
		}()

		codec := track.Codec()
		if codec.Name == webrtc.VP8 {
			i, err := ivfwriter.NewWith(os.Stdout)
			if err != nil {
				log.Fatal(err)
			}

			svc.saveToDisk(i, track)
		}
	})

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	svc.conn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Create a video track
	vp8Track, err := svc.conn.NewTrack(webrtc.DefaultPayloadTypeVP8, rand.Uint32(), "video", "pion")
	if err != nil {
		log.Fatal(err)
	}
	_, err = svc.conn.AddTrack(vp8Track)
	if err != nil {
		log.Fatal(err)
	}

	// Wait for the offer to be pasted
	offer := webrtc.SessionDescription{}
	key := <-svc.recv
	Decode(key, &offer)

	// Set the remote SessionDescription
	err = svc.conn.SetRemoteDescription(offer)
	if err != nil {
		log.Fatal(err)
	}

	// Create answer
	answer, err := svc.conn.CreateAnswer(nil)
	if err != nil {
		log.Fatal(err)
	}

	// Sets the LocalDescription, and starts our UDP listeners
	err = svc.conn.SetLocalDescription(answer)
	if err != nil {
		log.Fatal(err)
	}

	svc.sndr <- Encode(answer)
	<-svc.ready

	// open port for FFMPEG
	udpAddr, err := net.ResolveUDPAddr("udp4", "0.0.0.0:1234")
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}

	mtu := 8192
	buffer := make([]byte, mtu)

	var p codecs.VP8Packet
	sb := samplebuilder.New(50, &p)

	i := 0
	for {
		n, _, err := ln.ReadFromUDP(buffer)
		//n, err := bufio.NewReader(os.Stdin).Read(buffer)
		if err != nil && err == io.EOF {
			log.Println(err)
			break
		}

		remoteBuffer := make([]byte, n)
		copy(remoteBuffer, buffer)

		packet := new(rtp.Packet)
		err = packet.Unmarshal(remoteBuffer)

		if i%3 == 0 {
			sample := sb.Pop()
			if sample != nil {
				s := *sample
				vp8Track.WriteSample(s)
			}
		}

		sb.Push(packet)

		i++
	}
}

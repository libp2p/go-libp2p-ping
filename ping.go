package ping

import (
	"bytes"
	"errors"
	"io"
	"time"

	u "github.com/ipfs/go-ipfs-util"
	peer "github.com/ipfs/go-libp2p-peer"
	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p/p2p/host"
	inet "github.com/libp2p/go-libp2p/p2p/net"
	context "golang.org/x/net/context"
)

var log = logging.Logger("ping")

const PingSize = 32

const ID = "/ipfs/ping/1.0.0"

const pingTimeout = time.Second * 60

type PingService struct {
	Host host.Host
}

func NewPingService(h host.Host) *PingService {
	ps := &PingService{h}
	h.SetStreamHandler(ID, ps.PingHandler)
	return ps
}

func (p *PingService) PingHandler(s inet.Stream) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := make([]byte, PingSize)

	timer := time.NewTimer(pingTimeout)
	defer timer.Stop()

	go func() {
		select {
		case <-timer.C:
		case <-ctx.Done():
		}

		s.Close()
	}()

	for {
		_, err := io.ReadFull(s, buf)
		if err != nil {
			log.Debug(err)
			return
		}

		_, err = s.Write(buf)
		if err != nil {
			log.Debug(err)
			return
		}

		timer.Reset(pingTimeout)
	}
}

func (ps *PingService) Ping(ctx context.Context, p peer.ID) (<-chan time.Duration, error) {
	s, err := ps.Host.NewStream(ctx, ID, p)
	if err != nil {
		return nil, err
	}

	out := make(chan time.Duration)
	go func() {
		defer close(out)
		defer s.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				t, err := ping(s)
				if err != nil {
					log.Debugf("ping error: %s", err)
					return
				}

				ps.Host.Peerstore().RecordLatency(p, t)
				select {
				case out <- t:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func ping(s inet.Stream) (time.Duration, error) {
	buf := make([]byte, PingSize)
	u.NewTimeSeededRand().Read(buf)

	before := time.Now()
	_, err := s.Write(buf)
	if err != nil {
		return 0, err
	}

	rbuf := make([]byte, PingSize)
	_, err = io.ReadFull(s, rbuf)
	if err != nil {
		return 0, err
	}

	if !bytes.Equal(buf, rbuf) {
		return 0, errors.New("ping packet was incorrect!")
	}

	return time.Since(before), nil
}

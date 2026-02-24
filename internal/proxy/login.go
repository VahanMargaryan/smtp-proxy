package proxy

import "github.com/emersion/go-sasl"

// loginServer implements sasl.Server for the LOGIN authentication mechanism.
// This is designed for single-goroutine use per the go-sasl Server interface contract.
type loginServer struct {
	username string
	step     int
	validate func(username, password string) error
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step++
		return []byte("Username:"), false, nil
	case 1:
		s.username = string(response)
		s.step++
		return []byte("Password:"), false, nil
	case 2:
		s.step++
		err = s.validate(s.username, string(response))
		return nil, true, err
	default:
		return nil, false, sasl.ErrUnexpectedClientResponse
	}
}

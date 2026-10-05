// Package daisen provides a trace visualization server for Akita simulations.
package daisen

import "github.com/sarchlab/akita/v5/daisen/internal/httpapi"

type ProgressBar = httpapi.ProgressBar
type Server = httpapi.Server

func NewReplayServer(sqliteFile, addr string) *Server {
	return httpapi.NewReplayServer(sqliteFile, addr)
}

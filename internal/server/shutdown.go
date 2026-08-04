package server

func (s *Server) Close() {
	s.dropSessions()
}

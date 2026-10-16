package app

import "sync"

// startupResult sincroniza a primeira consulta de autenticação com o fim do
// startup inteiro: RefreshAuth também depende dos managers montados após o cofre.
// É criado por NewApp, antes de o Wails poder invocar qualquer binding.
// Fixtures que montam App diretamente não executam esse ciclo e usam nil.
type startupResult struct {
	done chan struct{}
	once sync.Once
	err  error // escrito antes de fechar done; lido apenas após recebê-lo
}

func newStartupResult() *startupResult {
	return &startupResult{done: make(chan struct{})}
}

func (s *startupResult) finish(err error) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.err = err
		close(s.done)
	})
}

func (s *startupResult) wait() error {
	if s == nil {
		return nil
	}
	<-s.done
	return s.err
}

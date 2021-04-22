package main

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

func lerCapsulas() map[string]string {
	f, err := os.Open("capsulas.csv")
	if err != nil {
		log.Fatal("Erro abrindo lista de capsulas: %v", err)
	}
	cr := csv.NewReader(f)
	cs := make(map[string]string)
	for {
		r, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Erro lendo lista de capsulas: %v", err)
		}
		if len(r) < 2 {
			log.Fatal("Registro de capsula inválido: %v", r)
		}
		if cs[r[1]] != "" {
			log.Fatal("Capsula com nome duplicado: %v", r[1])
		}
		cs[r[1]] = r[0]
	}
	return cs
}

func lerHistoricos() [][]string {
	h, err := ler("gemini://em-orbita.com/historico.csv")
	if err != nil {
		log.Fatalf("Erro lendo histórico: %v", err)
	}
	cr := csv.NewReader(strings.NewReader(h))
	var hs [][]string
	for {
		r, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Erro lendo lista de históricos: %v", err)
		}
		if len(r) < 3 {
			log.Fatal("Registro de histórico inválido: %v", r)
		}
		hs = append(hs, r)
	}
	return hs
}

func atualizar(h [][]string, cs map[string]string) [][]string {
	var (
		nh,oh [][]string
	)
	for _, hh := range h {
		c := cs[hh[0]]
		if c == "" {
			log.Printf("Capsula %s não encontrada\n", hh[0])
			oh = append(oh, hh)
			continue
		}
		p, err := ler(c)
		if err != nil {
			log.Printf("Erro lendo capsula %s: %v\n", hh[0], err)
			oh = append(oh, append(hh, c))
			continue
		}
		h := md5.New()
		io.WriteString(h, p)
		ns := fmt.Sprintf("%x", h.Sum(nil))
		if hh[1] != ns {
			log.Printf("Capsula %s atualizada\n", hh[0])
			nh = append(nh, []string{hh[0], ns, time.Now().Format("2006-01-02"), c})
 		} else {
 			log.Printf("Capsula %s não foi atualizada\n", hh[0])
 			oh = append(oh, append(hh, c))
		}
		delete(cs, hh[0])
	}
	// Agora atualizar as novas capsulas.
	for n, u := range cs {
		p, err := ler(u)
		if err != nil {
			log.Printf("Erro lendo capsula %s: %v\n", n, err)
			oh = append(oh, []string{n, "0", "0", u})
			continue
		}
		h := md5.New()
		io.WriteString(h, p)
		ns := fmt.Sprintf("%x", h.Sum(nil))
		log.Printf("Capsula %s atualizada\n", n)
		nh = append(nh, []string{n, ns, time.Now().Format("2006-01-02"), u})
	}
	return append(nh, oh...)
}

func ler(u string) (string, error){
	u = strings.TrimSpace(u)
	ur, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("Erro processando URL: %v", err)
	}
	s := ur.Host
	if ur.Port() == "" {
		s += ":1965"
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Duration(5) * time.Second}, "tcp", s, &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err != nil {
		return "", fmt.Errorf("Erro de conexão: %v", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(ur.String() + "\r\n"))
	if err != nil {
		return "", fmt.Errorf("Erro de envio: %v", err)
	}

	r, err := ioutil.ReadAll(conn)
	if err != nil {
		return "", fmt.Errorf("Erro de leitura: %v", err)
	}

	resp := strings.SplitN(string(r), "\r\n", 2)
	if len(resp) < 2 || len(resp[0]) == 0 {
		return "", fmt.Errorf("Resposta inesperada: %v", r)
	}
	if resp[0][0] == '2' {
		// Leitura correta.
		return resp[1], nil
	} else if resp[0][0] == '3' {
		p := strings.SplitN(resp[0], " ", 2)
		if len(p) < 2 {
			return "", fmt.Errorf("Redirecionamento inesperado: %v", resp[0])
		}
		// Redirecionamento.
		return ler(p[1])
	}
	return "", fmt.Errorf("Erro do servidor: %s", r)
}

func escrever(h [][]string) {
	escreverHistorico(h)
	escreverPagina(h)
}

func escreverHistorico(h [][]string) {
	f, err := os.OpenFile("orbita/historico.csv", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Erro abrindo novo arquivo de historico: %v", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	for _, d := range h {
		if len(d) < 3 {
			log.Fatal("Registro de historico invalido: %v", d)
		}
		if err := w.Write(d[:3]); err != nil {
			log.Fatal("Erro escrevendo novo historico: %v", err)
		}
	}
	w.Flush()
}

func escreverPagina(h [][]string) {
	c, err := ioutil.ReadFile("cabecalho.gmi")
	if err != nil {
		log.Fatalf("Erro lendo cabecalho.gmi: %v", err)
	}
	var s bytes.Buffer
	s.Write(c)
	for _, d := range h {
		if len(d) < 4 {
			continue
		}
		s.WriteString(fmt.Sprintf("=> %s %s - %s\n", d[3], d[0], d[2]))
	}
	r, err := ioutil.ReadFile("rodape.gmi")
	if err != nil {
		log.Fatalf("Erro lendo rodape.gmi: %v", err)
	}
	s.Write(r)
	f, err := os.OpenFile("orbita/index.gmi", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Erro abrindo novo index: %v", err)
	}
	defer f.Close()
	f.Write(s.Bytes())
}


func main() {
	escrever(atualizar(lerHistoricos(), lerCapsulas()))
}
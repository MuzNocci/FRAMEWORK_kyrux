package smtp

// Testes de Client.Send contra um servidor SMTP real na outra ponta do
// socket (TCP + TLS de verdade — não é mock do próprio código). Como um
// servidor SMTPS público não está disponível em CI, o "servidor real" aqui
// é um listener TLS/SMTP mínimo subido em loopback, do mesmo jeito que
// core_mail_smtp_test.go usa um Mailpit real em vez de mock — só que sem a
// dependência de Docker, já que o objetivo é provar que o handshake TLS
// implícito (porta 465) e o envio da mensagem completam de ponta a ponta.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	kymail "kyrux/core/mail"
)

// generateTestCert cria um certificado autoassinado válido para 127.0.0.1,
// só para os testes deste pacote confiarem no servidor fake via
// Client.tlsConfig (nunca usado em produção).
func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("criar certificado: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

// runFakeSMTP fala o mínimo de SMTP necessário para aceitar uma mensagem
// inteira (EHLO/AUTH/MAIL/RCPT/DATA/QUIT) sobre a conexão já aceita —
// chamado tanto pelo listener TLS (SMTPS) quanto pelo listener em texto
// plano (regressão do caminho sem TLS).
func runFakeSMTP(conn net.Conn, onData func(data string)) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(s string) { conn.Write([]byte(s + "\r\n")) }
	write("220 fake.smtp ESMTP")

	var inData bool
	var dataBuf strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				write("250 OK: mensagem aceita")
				onData(dataBuf.String())
				continue
			}
			dataBuf.WriteString(line + "\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			write("250-fake.smtp saudando")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH"):
			write("235 autenticado")
		case strings.HasPrefix(upper, "MAIL FROM"):
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			write("250 OK")
		case upper == "DATA":
			inData = true
			write("354 pode mandar, termine com <CRLF>.<CRLF>")
		case upper == "QUIT":
			write("221 até mais")
			return
		default:
			write("500 comando desconhecido")
		}
	}
}

// startFakeSMTPS sobe um listener TLS (SMTPS de verdade — handshake TLS
// antes de qualquer byte SMTP) e devolve o endereço e o certificado usado,
// para o teste montar um tls.Config que confia nele.
func startFakeSMTPS(t *testing.T, onData func(data string)) (addr string, cert tls.Certificate) {
	t.Helper()

	cert = generateTestCert(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		runFakeSMTP(conn, onData)
	}()

	return ln.Addr().String(), cert
}

// startFakePlainSMTP sobe um listener TCP em texto plano — usado para
// confirmar que portas diferentes de 465 continuam sem TLS implícito
// (regressão do comportamento anterior à porta 465).
func startFakePlainSMTP(t *testing.T, onData func(data string)) (addr string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		runFakeSMTP(conn, onData)
	}()

	return ln.Addr().String()
}

func TestClientSendPorta465UsaTLSImplicito(t *testing.T) {
	var received string
	addrFull, cert := startFakeSMTPS(t, func(data string) { received = data })

	host, port, err := net.SplitHostPort(addrFull)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	pool := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pool.AddCert(leaf)

	client := &Client{
		host:        host,
		port:        port, // porta efêmera do listener de teste, não a 465 real
		username:    "user@example.com",
		password:    "senha",
		implicitTLS: true, // decidido normalmente em Configure() por port=="465"; aqui é setado direto porque o teste não pode bindar a porta 465 privilegiada
		tlsConfig:   &tls.Config{ServerName: host, RootCAs: pool},
	}

	msg := kymail.Message{
		From:    "remetente@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto smtps",
		Text:    "corpo em texto puro",
		HTML:    "<p>corpo em <b>HTML</b></p>",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for received == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(received, "assunto smtps") {
		t.Errorf("esperava o assunto no corpo recebido pelo servidor fake, recebeu: %q", received)
	}
	if !strings.Contains(received, "corpo em texto puro") {
		t.Errorf("esperava o corpo em texto puro na mensagem recebida, recebeu: %q", received)
	}
}

func TestClientSendPortaDiferenteDe465NaoUsaTLSImplicito(t *testing.T) {
	var received string
	addrFull := startFakePlainSMTP(t, func(data string) { received = data })

	host, port, err := net.SplitHostPort(addrFull)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	client := &Client{host: host, port: port} // porta efêmera != "465", useTLS=false

	msg := kymail.Message{
		From:    "remetente@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto texto plano",
		Text:    "corpo simples",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for received == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(received, "assunto texto plano") {
		t.Errorf("esperava o assunto na mensagem recebida em texto plano, recebeu: %q", received)
	}
}

func TestClientSendComReplyToIncluiHeader(t *testing.T) {
	var received string
	addrFull := startFakePlainSMTP(t, func(data string) { received = data })

	host, port, err := net.SplitHostPort(addrFull)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	client := &Client{host: host, port: port}

	msg := kymail.Message{
		From:    "no-reply@kyrux.teste",
		ReplyTo: "visitante@example.com",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto com reply-to",
		Text:    "corpo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for received == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(received, "Reply-To: visitante@example.com") {
		t.Errorf("esperava o header Reply-To na mensagem recebida, recebeu: %q", received)
	}
}

func TestClientSendSemReplyToNaoIncluiHeader(t *testing.T) {
	var received string
	addrFull := startFakePlainSMTP(t, func(data string) { received = data })

	host, port, err := net.SplitHostPort(addrFull)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	client := &Client{host: host, port: port}

	msg := kymail.Message{
		From:    "no-reply@kyrux.teste",
		To:      []string{"destinatario@kyrux.teste"},
		Subject: "assunto sem reply-to",
		Text:    "corpo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for received == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if strings.Contains(received, "Reply-To:") {
		t.Errorf("não esperava header Reply-To sem ReplyTo na mensagem, recebeu: %q", received)
	}
}

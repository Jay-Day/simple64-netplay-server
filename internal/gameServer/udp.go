package gameserver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

type GameData struct {
	SyncValues      map[uint32][]byte
	PlayerAddresses []*net.UDPAddr
	BufferSize      []uint32
	BufferHealth    []int32
	Inputs          []map[uint32]uint32
	Plugin          []map[uint32]byte
	PendingInput    []uint32
	CountLag        []uint32
	PendingPlugin   []byte
	PlayerAlive     []bool
	LeadCount       uint32
	Status          byte
}

const (
	KeyInfoClient           = 0
	KeyInfoServer           = 1
	PlayerInputRequest      = 2
	KeyInfoServerGratuitous = 3
	CP0Info                 = 4
)

const (
	StatusDesync              = 1
	DisconnectTimeoutS        = 30
	NoRegID                   = 255
	InputDataMax       uint32 = 5000
)

// returns true if v is bigger than w (accounting for uint32 wrap around).
func uintLarger(v uint32, w uint32) bool {
	return (w - v) > (math.MaxUint32 / 2) //nolint:gomnd
}

func (g *GameServer) getPlayerNumberByID(regID uint32) (byte, error) {
	var i byte
	for i = 0; i < 4; i++ {
		v, ok := g.Registrations[i]
		if ok {
			if v.RegID == regID {
				return i, nil
			}
		}
	}
	return NoRegID, fmt.Errorf("could not find ID")
}

func (g *GameServer) fillInput(playerNumber byte, count uint32) {
	_, inputExists := g.GameData.Inputs[playerNumber][count]
	delete(g.GameData.Inputs[playerNumber], count-InputDataMax) // no need to keep old input data
	delete(g.GameData.Plugin[playerNumber], count-InputDataMax) // no need to keep old input data
	if !inputExists {
		g.GameData.Inputs[playerNumber][count] = g.GameData.PendingInput[playerNumber]
		g.GameData.Plugin[playerNumber][count] = g.GameData.PendingPlugin[playerNumber]
	}
}

func (g *GameServer) sendUDPInput(count uint32, addr *net.UDPAddr, playerNumber byte, spectator bool, sendingPlayerNumber byte) uint32 {
	buffer := make([]byte, 512) //nolint:gomnd
	var countLag uint32
	if uintLarger(count, g.GameData.LeadCount) {
		g.Logger.Error(fmt.Errorf("bad count lag"), "count is larger than LeadCount", "count", count, "LeadCount", g.GameData.LeadCount, "playerNumber", playerNumber)
	} else {
		countLag = g.GameData.LeadCount - count
	}
	if sendingPlayerNumber == NoRegID { // if the incoming packet was KeyInfoClient, the regID isn't included in the packet
		sendingPlayerNumber = playerNumber
		buffer[0] = KeyInfoServerGratuitous // client will ignore countLag value in this case
	} else {
		buffer[0] = KeyInfoServer
	}
	buffer[1] = playerNumber
	buffer[2] = g.GameData.Status
	buffer[3] = uint8(countLag)
	currentByte := 5
	start := count
	end := start + g.GameData.BufferSize[sendingPlayerNumber]
	_, ok := g.GameData.Inputs[playerNumber][count]                                              // check if input exists for this count
	for (currentByte < 500) && ((!spectator && countLag == 0 && uintLarger(end, count)) || ok) { //nolint:gomnd
		binary.BigEndian.PutUint32(buffer[currentByte:], count)
		currentByte += 4
		g.fillInput(playerNumber, count)
		binary.BigEndian.PutUint32(buffer[currentByte:], g.GameData.Inputs[playerNumber][count])
		currentByte += 4
		buffer[currentByte] = g.GameData.Plugin[playerNumber][count]
		currentByte++
		count++
		_, ok = g.GameData.Inputs[playerNumber][count] // check if input exists for this count
	}
	buffer[4] = uint8(count - start) // number of counts in packet
	if currentByte > 5 {             //nolint:gomnd
		_, err := g.UDPListener.WriteToUDP(buffer[0:currentByte], addr)
		if err != nil {
			g.Logger.Error(err, "could not send input")
		}
	}
	return countLag
}

func (g *GameServer) processUDP(addr *net.UDPAddr, buf []byte) {
	playerNumber := buf[1]
	if buf[0] == KeyInfoClient {
		g.GameData.PlayerAddresses[playerNumber] = addr
		count := binary.BigEndian.Uint32(buf[2:])
		if uintLarger(count, g.GameData.LeadCount) {
			g.GameData.LeadCount = count
		}
		g.GameData.PendingInput[playerNumber] = binary.BigEndian.Uint32(buf[6:])
		g.GameData.PendingPlugin[playerNumber] = buf[10]

		for i := 0; i < 4; i++ {
			if g.GameData.PlayerAddresses[i] != nil {
				g.sendUDPInput(count, g.GameData.PlayerAddresses[i], playerNumber, true, NoRegID)
			}
		}
	} else if buf[0] == PlayerInputRequest {
		regID := binary.BigEndian.Uint32(buf[2:])
		count := binary.BigEndian.Uint32(buf[6:])
		spectator := buf[10]
		if uintLarger(count, g.GameData.LeadCount) && spectator == 0 {
			g.GameData.LeadCount = count
		}
		sendingPlayerNumber, err := g.getPlayerNumberByID(regID)
		countLag := g.sendUDPInput(count, addr, playerNumber, spectator != 0, sendingPlayerNumber)
		if err != nil {
			g.Logger.Error(err, "could not process request", "regID", regID)
			return
		}
		g.GameData.BufferHealth[sendingPlayerNumber] = int32(buf[11])
		g.GameData.PlayerAlive[sendingPlayerNumber] = true
		g.GameData.CountLag[sendingPlayerNumber] = countLag
	} else if buf[0] == CP0Info {
		if g.GameData.Status&StatusDesync == 0 {
			viCount := binary.BigEndian.Uint32(buf[1:])
			_, ok := g.GameData.SyncValues[viCount]
			if !ok {
				if len(g.GameData.SyncValues) > 50 { //nolint:gomnd // no need to keep old sync hashes
					g.GameData.SyncValues = make(map[uint32][]byte)
				}
				g.GameData.SyncValues[viCount] = buf[5:133]
			} else if !bytes.Equal(g.GameData.SyncValues[viCount], buf[5:133]) {
				g.GameData.Status |= StatusDesync
				g.Logger.Error(fmt.Errorf("desync"), "game has desynced")
			}
		}
	}
}

func (g *GameServer) watchUDP() {
	for {
		buf := make([]byte, 1024) //nolint:gomnd
		_, addr, err := g.UDPListener.ReadFromUDP(buf)
		if err != nil && !g.isConnClosed(err) {
			g.Logger.Error(err, "error from UdpListener")
			continue
		} else if g.isConnClosed(err) {
			return
		}
		g.processUDP(addr, buf)
	}
}

func (g *GameServer) ManageBuffer() {
	for {
		if !g.Running {
			g.Logger.Info("done managing buffers")
			return
		}
		for i := 0; i < 4; i++ {
			if g.GameData.BufferHealth[i] != -1 {
				if g.GameData.BufferHealth[i] > BufferTarget && g.GameData.BufferSize[i] > 0 {
					g.GameData.BufferSize[i]--
					// g.Logger.Info("reducing buffer size", "player", i, "bufferSize", g.GameData.BufferSize[i])
				} else if g.GameData.BufferHealth[i] < BufferTarget {
					g.GameData.BufferSize[i]++
					// g.Logger.Info("increasing buffer size", "player", i, "bufferSize", g.GameData.BufferSize[i])
				}
			}
		}
		time.Sleep(time.Second * 5) //nolint:gomnd
	}
}

func (g *GameServer) ManagePlayers() {
	for {
		if len(g.GameData.Inputs[0]) == 0 { // wait for game to start
			time.Sleep(time.Second * DisconnectTimeoutS)
			continue
		}
		playersActive := false // used to check if anyone is still around
		var i byte
		for i = 0; i < 4; i++ {
			_, ok := g.Registrations[i]
			if ok {
				if g.GameData.PlayerAlive[i] {
					g.Logger.Info("player status", "player", i, "regID", g.Registrations[i].RegID, "bufferSize", g.GameData.BufferSize[i], "bufferHealth", g.GameData.BufferHealth[i], "countLag", g.GameData.CountLag[i], "address", g.GameData.PlayerAddresses[i])
					playersActive = true
				} else {
					g.Logger.Info("play disconnected UDP", "player", i, "regID", g.Registrations[i].RegID, "address", g.GameData.PlayerAddresses[i])
					g.GameData.Status |= (0x1 << (i + 1)) //nolint:gomnd
					delete(g.Registrations, i)
				}
			}
			g.GameData.PlayerAlive[i] = false
		}
		if !playersActive {
			g.Logger.Info("no more players, closing room", "playTime", time.Since(g.StartTime).String())
			g.CloseServers()
			g.Running = false
			return
		}
		time.Sleep(time.Second * DisconnectTimeoutS)
	}
}

func (g *GameServer) createUDPServer() int {
	var err error
	g.UDPListener, err = net.ListenUDP("udp", &net.UDPAddr{Port: g.Port})
	if err != nil {
		g.Logger.Error(err, "failed to create UDP server")
		return 0
	}
	g.Logger.Info("Created UDP server", "port", g.Port)

	g.GameData.PlayerAddresses = make([]*net.UDPAddr, 4) //nolint:gomnd
	g.GameData.BufferSize = []uint32{3, 3, 3, 3}
	g.GameData.BufferHealth = []int32{-1, -1, -1, -1}
	g.GameData.Inputs = make([]map[uint32]uint32, 4) //nolint:gomnd
	for i := 0; i < 4; i++ {
		g.GameData.Inputs[i] = make(map[uint32]uint32)
	}
	g.GameData.Plugin = make([]map[uint32]byte, 4) //nolint:gomnd
	for i := 0; i < 4; i++ {
		g.GameData.Plugin[i] = make(map[uint32]byte)
	}
	g.GameData.PendingInput = make([]uint32, 4) //nolint:gomnd
	g.GameData.PendingPlugin = make([]byte, 4)  //nolint:gomnd
	g.GameData.SyncValues = make(map[uint32][]byte)
	g.GameData.PlayerAlive = make([]bool, 4) //nolint:gomnd
	g.GameData.CountLag = make([]uint32, 4)  //nolint:gomnd

	go g.watchUDP()
	return g.Port
}

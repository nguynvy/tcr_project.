package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"

	"tcr_project/auth"
)

type PlayerConn struct {
	conn     net.Conn
	username string
}

var (
	waitingPlayers []PlayerConn
	mu             sync.Mutex
	AllTroops      map[string]Troop
)

type Tower struct {
	Name string
	HP   int
	ATK  int
	DEF  int
	CRIT float64
}

type Troop struct {
	Name    string
	HP      int
	ATK     int
	DEF     int
	MANA    int
	EXP     int
	Special string
}

type PlayerState struct {
	Conn      net.Conn
	Username  string
	Mana      int
	KingTower Tower
	Guard1    Tower
	Guard2    Tower
	Troops    []Troop
}

type GameState struct {
	P1, P2 PlayerState
	P1Turn bool
	Over   bool
}

func main() {
	err := loadTroopSpecs("server/assets/specs.json")
	if err != nil {
		fmt.Println("Không load được troop specs:", err)
		return
	}

	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Lỗi khởi tạo server:", err)
		return
	}
	defer ln.Close()
	fmt.Println("Server đang chạy tại cổng 8080...")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Lỗi kết nối:", err)
			continue
		}
		go handleClient(conn)
	}
}

func loadTroopSpecs(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	AllTroops = make(map[string]Troop)
	for name, val := range raw["troops"] {
		AllTroops[strings.ToLower(name)] = Troop{
			Name:    name,
			HP:      int(val["HP"].(float64)),
			ATK:     int(val["ATK"].(float64)),
			DEF:     int(val["DEF"].(float64)),
			MANA:    int(val["MANA"].(float64)),
			EXP:     int(val["EXP"].(float64)),
			Special: val["Special"].(string),
		}
	}
	return nil
}

func handleClient(conn net.Conn) {
	fmt.Fprintln(conn, "Chào mừng đến với TCR Server!")

	reader := bufio.NewReader(conn)
	loginData, _ := reader.ReadString('\n')
	loginData = strings.TrimSpace(loginData)
	parts := strings.Split(loginData, "|")
	if len(parts) != 2 {
		fmt.Fprintln(conn, "Sai định dạng đăng nhập!")
		conn.Close()
		return
	}

	username := parts[0]
	password := parts[1]

	if valid, _ := auth.CheckLogin(username, password); !valid {
		fmt.Fprintln(conn, "Sai tài khoản hoặc mật khẩu.")
		conn.Close()
		return
	}

	fmt.Println("Người dùng đăng nhập:", username)
	fmt.Fprintln(conn, "Đăng nhập thành công!")

	mu.Lock()
	waitingPlayers = append(waitingPlayers, PlayerConn{conn, username})
	if len(waitingPlayers) >= 2 {
		player1 := waitingPlayers[0]
		player2 := waitingPlayers[1]
		waitingPlayers = waitingPlayers[2:]
		mu.Unlock()
		go startMatch(player1, player2)
	} else {
		mu.Unlock()
		fmt.Fprintln(conn, "Đang chờ người chơi khác kết nối...")
	}
}

func NewPlayerState(username string, conn net.Conn) PlayerState {
	return PlayerState{
		Conn:      conn,
		Username:  username,
		Mana:      5,
		KingTower: Tower{"King", 2000, 500, 300, 0.1},
		Guard1:    Tower{"Guard1", 1000, 300, 100, 0.05},
		Guard2:    Tower{"Guard2", 1000, 300, 100, 0.05},
	}
}

func startMatch(p1Conn, p2Conn PlayerConn) {
	fmt.Println("Bắt đầu trận đấu giữa", p1Conn.username, "và", p2Conn.username)

	p1 := NewPlayerState(p1Conn.username, p1Conn.conn)
	p2 := NewPlayerState(p2Conn.username, p2Conn.conn)

	game := GameState{
		P1:     p1,
		P2:     p2,
		P1Turn: true,
	}

	go startManaRegen(&game.P1, &game)
	go startManaRegen(&game.P2, &game)

	go handlePlayer(&game, p1)
	go handlePlayer(&game, p2)

	//Set up 3 phút cho trận đấu
	go startTimer(&game)
}

func startTimer(game *GameState) {
	timer := time.NewTicker(60 * time.Second)
	defer timer.Stop()

	<-timer.C
	fmt.Fprintln(game.P1.Conn, "⏰ Thời gian đã hết! Trận đấu kết thúc!")
	fmt.Fprintln(game.P2.Conn, "⏰ Thời gian đã hết! Trận đấu kết thúc!")

	// Đánh dấu trận đấu đã kết thúc
	game.Over = true

	// Kiểm tra trạng thái King Tower để xác định kết quả
	if game.P1.KingTower.HP > 0 && game.P2.KingTower.HP <= 0 {
		fmt.Fprintln(game.P1.Conn, "🏆 Bạn thắng vì đối thủ đã mất King Tower!")
		fmt.Fprintln(game.P2.Conn, "💥 Bạn thua vì mất King Tower!")
	} else if game.P2.KingTower.HP > 0 && game.P1.KingTower.HP <= 0 {
		fmt.Fprintln(game.P2.Conn, "🏆 Bạn thắng vì đối thủ đã mất King Tower!")
		fmt.Fprintln(game.P1.Conn, "💥 Bạn thua vì mất King Tower!")
	} else if game.P1.KingTower.HP > 0 && game.P2.KingTower.HP > 0 {
		if game.P1.KingTower.HP > game.P2.KingTower.HP {
			fmt.Fprintln(game.P1.Conn, "🏆 Bạn thắng vì King Tower còn nhiều HP hơn!")
			fmt.Fprintln(game.P2.Conn, "💥 Bạn thua vì King Tower ít HP hơn!")
		} else if game.P2.KingTower.HP > game.P1.KingTower.HP {
			fmt.Fprintln(game.P2.Conn, "🏆 Bạn thắng vì King Tower còn nhiều HP hơn!")
			fmt.Fprintln(game.P1.Conn, "💥 Bạn thua vì King Tower ít HP hơn!")
		} else {
			fmt.Fprintln(game.P1.Conn, "🤝 Trận đấu hoà! Hai bên có cùng lượng HP.")
			fmt.Fprintln(game.P2.Conn, "🤝 Trận đấu hoà! Hai bên có cùng lượng HP.")
		}
	} else {
		fmt.Fprintln(game.P1.Conn, "🤝 Trận đấu hoà!")
		fmt.Fprintln(game.P2.Conn, "🤝 Trận đấu hoà!")
	}
	fmt.Fprintln(game.P1.Conn, "Gõ 'replay' để chơi lại hoặc 'quit' để thoát.")
	fmt.Fprintln(game.P2.Conn, "Gõ 'replay' để chơi lại hoặc 'quit' để thoát.")
}

func handlePlayer(game *GameState, player PlayerState) {
	reader := bufio.NewReader(player.Conn)
	for {
		// Nếu game đã kết thúc, chờ người chơi nhập replay hoặc quit
		if game.Over {
			fmt.Fprint(player.Conn, "🔚 Trận đấu đã kết thúc. Gõ `replay` để chơi lại hoặc `quit` để thoát:\n")
			line, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("Ngắt kết nối người chơi", player.Username)
				return
			}
			line = strings.TrimSpace(strings.ToLower(line))

			if line == "replay" {
				fmt.Fprintln(player.Conn, "🔁 Đang tìm đối thủ mới để chơi lại...")
				mu.Lock()
				waitingPlayers = append(waitingPlayers, PlayerConn{player.Conn, player.Username})
				if len(waitingPlayers) >= 2 {
					p1 := waitingPlayers[0]
					p2 := waitingPlayers[1]
					waitingPlayers = waitingPlayers[2:]
					mu.Unlock()
					go startMatch(p1, p2)
				} else {
					mu.Unlock()
					fmt.Fprintln(player.Conn, "⏳ Đang chờ người chơi khác...")
				}
				return
			} else if line == "quit" {
				fmt.Fprintln(player.Conn, "👋 Tạm biệt! Cảm ơn bạn đã chơi TCR.")
				player.Conn.Close()
				return
			} else {
				fmt.Fprintln(player.Conn, "⚠️ Lệnh không hợp lệ. Gõ `replay` để chơi lại hoặc `quit` để thoát.")
				continue
			}
		}

		// Xử lý lệnh khi trận chưa kết thúc
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Ngắt kết nối người chơi", player.Username)
			return
		}
		line = strings.TrimSpace(line)

		if !game.isPlayerTurn(player.Username) {
			fmt.Fprintln(player.Conn, "❌ Chưa đến lượt bạn!")
			continue
		}

		valid := game.processCommand(player.Username, line)
		if !valid {
			fmt.Fprintln(player.Conn, "⚠️ Lệnh không hợp lệ, vui lòng nhập lại.")
		}
	}
}

func (g *GameState) isPlayerTurn(username string) bool {
	if g.P1.Username == username {
		return g.P1Turn
	}
	return !g.P1Turn
}

func (g *GameState) getPlayerState(username string) *PlayerState {
	if g.P1.Username == username {
		return &g.P1
	}
	return &g.P2
}

func (g *GameState) getOpponentState(username string) *PlayerState {
	if g.P1.Username != username {
		return &g.P1
	}
	return &g.P2
}

func (g *GameState) processCommand(username string, cmd string) bool {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	attacker := g.getPlayerState(username)
	defender := g.getOpponentState(username)

	if g.Over {
		fmt.Fprintln(attacker.Conn, "Trận đấu đã kết thúc! Bạn không thể thực hiện hành động nữa.")
		return true
	}

	switch {
	case strings.HasPrefix(cmd, "summon"):
		parts := strings.Fields(cmd)
		if len(parts) != 2 {
			fmt.Fprintln(attacker.Conn, "Cú pháp đúng: summon <pawn/bishop/...>")
			return true
		}
		troopName := parts[1]
		troop, ok := AllTroops[troopName]
		if !ok {
			fmt.Fprintln(attacker.Conn, "Không tồn tại troop tên này!")
			return true
		}
		if attacker.Mana < troop.MANA {
			fmt.Fprintf(attacker.Conn, "Không đủ mana! Cần %d, bạn có %d\n", troop.MANA, attacker.Mana)
			return true
		}
		attacker.Mana -= troop.MANA
		attacker.Troops = append(attacker.Troops, troop)
		fmt.Fprintf(attacker.Conn, "✅ Triệu hồi %s thành công! Mana còn lại: %d\n", troop.Name, attacker.Mana)
		return true

	case strings.HasPrefix(cmd, "attack"):
		if len(attacker.Troops) == 0 {
			fmt.Fprintln(attacker.Conn, "⚠️ Bạn chưa có troop nào! Dùng: summon <pawn/rook/...>")
			return true
		}
		parts := strings.Fields(cmd)
		if len(parts) != 2 {
			fmt.Fprintln(attacker.Conn, "Cú pháp đúng: attack g1 / g2 / king")
			return true
		}
		target := parts[1]
		var tower *Tower
		var towerName string

		switch target {
		case "g1":
			tower = &defender.Guard1
			towerName = "Guard Tower 1"
		case "g2":
			if defender.Guard1.HP > 0 {
				fmt.Fprintln(attacker.Conn, "⚠️ Phải phá Guard Tower 1 trước khi tấn công Guard Tower 2!")
				return true
			}
			tower = &defender.Guard2
			towerName = "Guard Tower 2"
		case "king":
			if defender.Guard1.HP > 0 || defender.Guard2.HP > 0 {
				fmt.Fprintln(attacker.Conn, "⚠️ Phải phá cả Guard Tower 1 và 2 trước khi tấn công King Tower!")
				return true
			}
			tower = &defender.KingTower
			towerName = "King Tower"
		default:
			fmt.Fprintln(attacker.Conn, "⚠️ Mục tiêu không hợp lệ! Dùng: g1, g2, king")
			return true
		}

		troop := attacker.Troops[0]
		damage := troop.ATK - tower.DEF
		if damage < 0 {
			damage = 0
		}
		tower.HP -= damage
		if tower.HP < 0 {
			tower.HP = 0
		}
		attacker.Troops = attacker.Troops[1:]

		fmt.Fprintf(attacker.Conn, "⚔️ %s tấn công %s, gây %d sát thương! HP còn lại: %d\n", troop.Name, towerName, damage, tower.HP)
		fmt.Fprintf(defender.Conn, "💢 %s dùng %s tấn công %s của bạn! HP còn lại: %d\n", attacker.Username, troop.Name, towerName, tower.HP)

		if target == "king" && tower.HP <= 0 {
			fmt.Fprintln(attacker.Conn, "🏆 Bạn đã phá hủy King Tower và giành chiến thắng!")
			fmt.Fprintln(defender.Conn, "💀 King Tower của bạn đã bị phá! Bạn đã thua trận!")
			return true
		}

		g.P1Turn = !g.P1Turn
		fmt.Fprintln(g.P1.Conn, "🔄 Lượt tiếp theo.")
		fmt.Fprintln(g.P2.Conn, "🔄 Lượt tiếp theo.")
		return true

	case cmd == "defend":
		attacker.KingTower.HP += 50
		fmt.Fprintf(attacker.Conn, "🛡️ Bạn đã phòng thủ! HP King Tower tăng lên: %d\n", attacker.KingTower.HP)
		g.P1Turn = !g.P1Turn
		fmt.Fprintln(g.P1.Conn, "🔄 Lượt tiếp theo.")
		fmt.Fprintln(g.P2.Conn, "🔄 Lượt tiếp theo.")
		return true

	case cmd == "skill":
		attacker.KingTower.HP += 100
		fmt.Fprintf(attacker.Conn, "✨ Bạn dùng kỹ năng đặc biệt! HP King Tower tăng lên: %d\n", attacker.KingTower.HP)
		g.P1Turn = !g.P1Turn
		fmt.Fprintln(g.P1.Conn, "🔄 Lượt tiếp theo.")
		fmt.Fprintln(g.P2.Conn, "🔄 Lượt tiếp theo.")
		return true

	case cmd == "end":
		g.P1Turn = !g.P1Turn
		fmt.Fprintln(g.P1.Conn, "🔄 Lượt tiếp theo.")
		fmt.Fprintln(g.P2.Conn, "🔄 Lượt tiếp theo.")
		return true

	case cmd == "help":
		helpMsg := `
[ Hướng dẫn lệnh trong game ]
🔹 summon <tên quân>        : Gọi troop (pawn, rook,...)
🔹 attack g1/g2/king        : Tấn công tower đối thủ
🔹 defend                   : Hồi 50 HP King Tower
🔹 skill                    : Hồi 100 HP King Tower
🔹 end                      : Kết thúc lượt
🔹 help                     : Hiển thị hướng dẫn`
		fmt.Fprintln(attacker.Conn, helpMsg)
		return true
	}

	fmt.Fprintln(attacker.Conn, "⚠️ Lệnh không hợp lệ! Gõ `help` để xem hướng dẫn.")
	return false
}

func startManaRegen(player *PlayerState, game *GameState) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if game.Over {
			return
		}
		if player.Mana < 10 {
			player.Mana++
			fmt.Fprintf(player.Conn, "🔋 Mana tăng: %d\n", player.Mana)
		}
		<-ticker.C
	}
}

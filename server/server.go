package server

import (
	"fmt"
	"gopkg.in/mgo.v2"
	"github.com/Cristofori/kmud/database"
	"github.com/Cristofori/kmud/engine"
	"github.com/Cristofori/kmud/model"
	"github.com/Cristofori/kmud/session"
	"github.com/Cristofori/kmud/telnet"
	"github.com/Cristofori/kmud/utils"
	"net"
	"runtime/debug"
	"sort"
	"strconv"
	"time"
)

type Server struct {
	listener net.Listener
}

type wrappedConnection struct {
	telnet  *telnet.Telnet
	watcher *utils.WatchableReadWriter
}

func (s *wrappedConnection) Write(p []byte) (int, error) {
	return s.watcher.Write(p)
}

func (s *wrappedConnection) Read(p []byte) (int, error) {
	return s.watcher.Read(p)
}

func (s *wrappedConnection) Close() error {
	return s.telnet.Close()
}

func (s *wrappedConnection) LocalAddr() net.Addr {
	return s.telnet.LocalAddr()
}

func (s *wrappedConnection) RemoteAddr() net.Addr {
	return s.telnet.RemoteAddr()
}

func (s *wrappedConnection) SetDeadline(dl time.Time) error {
	return s.telnet.SetDeadline(dl)
}

func (s *wrappedConnection) SetReadDeadline(dl time.Time) error {
	return s.telnet.SetReadDeadline(dl)
}

func (s *wrappedConnection) SetWriteDeadline(dl time.Time) error {
	return s.telnet.SetWriteDeadline(dl)
}

func login(conn *wrappedConnection) *database.User {
	for {
		username := utils.GetUserInput(conn, "Username: ", utils.ColorModeNone)

		if username == "" {
			return nil
		}

		user := model.GetUserByName(username)

		if user == nil {
			utils.WriteLine(conn, "User not found", utils.ColorModeNone)
		} else if user.Online() {
			utils.WriteLine(conn, "That user is already online", utils.ColorModeNone)
		} else {
			attempts := 1
			conn.telnet.WillEcho()
			for {
				password := utils.GetRawUserInputSuffix(conn, "Password: ", "\r\n", utils.ColorModeNone)

				// TODO - Disabling password verification to make development easier
				if user.VerifyPassword(password) || true {
					break
				}

				if attempts >= 3 {
					utils.WriteLine(conn, "Too many failed login attempts", utils.ColorModeNone)
					conn.Close()
					panic("Booted user due to too many failed logins (" + user.GetName() + ")")
				}

				attempts++

				time.Sleep(2 * time.Second)
				utils.WriteLine(conn, "Invalid password", utils.ColorModeNone)
			}
			conn.telnet.WontEcho()

			return user
		}
	}
}

func newUser(conn *wrappedConnection) *database.User {
	for {
		name := utils.GetUserInput(conn, "Desired username: ", utils.ColorModeNone)

		if name == "" {
			return nil
		}

		user := model.GetUserByName(name)
		password := ""

		if user != nil {
			utils.WriteLine(conn, "That name is unavailable", utils.ColorModeNone)
		} else if err := utils.ValidateName(name); err != nil {
			utils.WriteLine(conn, err.Error(), utils.ColorModeNone)
		} else {
			conn.telnet.WillEcho()
			for {
				pass1 := utils.GetRawUserInputSuffix(conn, "Desired password: ", "\r\n", utils.ColorModeNone)

				if len(pass1) < 7 {
					utils.WriteLine(conn, "Passwords must be at least 7 letters in length", utils.ColorModeNone)
					continue
				}

				pass2 := utils.GetRawUserInputSuffix(conn, "Confirm password: ", "\r\n", utils.ColorModeNone)

				if pass1 != pass2 {
					utils.WriteLine(conn, "Passwords do not match", utils.ColorModeNone)
					continue
				}

				password = pass1

				break
			}
			conn.telnet.WontEcho()

			user = model.CreateUser(name, password)
			return user
		}
	}
}

func newPlayer(conn *wrappedConnection, user *database.User) *database.PlayerChar {
	// TODO: character slot limit
	const SizeLimit = 12
	for {
		name := user.GetInput("Desired character name: ")

		if name == "" {
			return nil
		}

		char := model.GetCharacterByName(name)

		if char != nil {
			user.WriteLine("That name is unavailable")
		} else if err := utils.ValidateName(name); err != nil {
			user.WriteLine(err.Error())
		} else {
			room := model.GetRooms()[0] // TODO: Better way to pick an initial character location
			return model.CreatePlayerCharacter(name, user, room)
		}
	}
}

func mainMenu() *utils.Menu {
	menu := utils.NewMenu("MUD")

	menu.AddAction("l", "Login")
	menu.AddAction("n", "New user")
	menu.AddAction("q", "Quit")

	return menu
}

func userMenu(user *database.User) *utils.Menu {
	chars := model.GetUserCharacters(user)

	menu := utils.NewMenu(user.GetName())
	menu.AddAction("l", "Logout")
	menu.AddAction("a", "Admin")
	menu.AddAction("n", "New character")
	if len(chars) > 0 {
		menu.AddAction("d", "Delete character")
	}

	// TODO: Sort character list

	for i, char := range chars {
		index := i + 1
		menu.AddActionData(index, char.GetName(), char.GetId())
	}

	return menu
}

func deleteMenu(user *database.User) *utils.Menu {
	chars := model.GetUserCharacters(user)

	menu := utils.NewMenu("Delete character")

	menu.AddAction("c", "Cancel")

	// TODO: Sort character list

	for i, char := range chars {
		index := i + 1
		menu.AddActionData(index, char.GetName(), char.GetId())
	}

	return menu
}

func adminMenu() *utils.Menu {
	menu := utils.NewMenu("Admin")
	menu.AddAction("u", "Users")
	return menu
}

func userAdminMenu() *utils.Menu {
	menu := utils.NewMenu("User Admin")

	users := model.GetUsers()
	sort.Sort(users)

	for i, user := range users {
		index := i + 1

		online := ""
		if user.Online() {
			online = "*"
		}

		menu.AddActionData(index, user.GetName()+online, user.GetId())
	}

	return menu
}

func userSpecificMenu(user *database.User) *utils.Menu {
	suffix := ""
	if user.Online() {
		suffix = "(Online)"
	} else {
		suffix = "(Offline)"
	}

	menu := utils.NewMenu("User: " + user.GetName() + " " + suffix)
	menu.AddAction("d", "Delete")

	if user.Online() {
		menu.AddAction("w", "Watch")
	}

	return menu
}

func handleConnection(conn *wrappedConnection) {
	defer conn.Close()

	var user *database.User
	var pc *database.PlayerChar

	defer func() {
		if r := recover(); r != nil {
			username := ""
			charname := ""

			if user != nil {
				user.SetOnline(false)
				username = user.GetName()
			}

			if pc != nil {
				pc.SetOnline(false)
				charname = pc.GetName()
			}

			debug.PrintStack()

			fmt.Printf("Lost connection to client (%v/%v): %v, %v\n",
				username,
				charname,
				conn.RemoteAddr(),
				r)
		}
	}()

	for {
		if user == nil {
			menu := mainMenu()
			choice, _ := menu.Exec(conn, utils.ColorModeNone)

			switch choice {
			case "l":
				user = login(conn)
			case "n":
				user = newUser(conn)
			case "":
				fallthrough
			case "q":
				utils.WriteLine(conn, "Take luck!", utils.ColorModeNone)
				conn.Close()
				return
			}

			if user == nil {
				continue
			}

			user.SetOnline(true)
			user.SetConnection(conn)

			conn.telnet.DoWindowSize()
			conn.telnet.DoTerminalType()

			conn.telnet.Listen(func(code telnet.TelnetCode, data []byte) {
				switch code {
				case telnet.WS:
					if len(data) != 4 {
						fmt.Println("Malformed window size data:", data)
						return
					}

					width := (255 * data[0]) + data[1]
					height := (255 * data[2]) + data[3]
					user.SetWindowSize(int(width), int(height))

				case telnet.TT:
					user.SetTerminalType(string(data))
				}
			})

		} else if pc == nil {
			menu := userMenu(user)
			choice, charId := menu.Exec(conn, user.GetColorMode())

			switch choice {
			case "":
				fallthrough
			case "l":
				user.SetOnline(false)
				user = nil
			case "a":
				adminMenu := adminMenu()
				for {
					choice, _ := adminMenu.Exec(conn, user.GetColorMode())
					if choice == "" {
						break
					} else if choice == "u" {
						for {
							userAdminMenu := userAdminMenu()
							choice, userId := userAdminMenu.Exec(conn, user.GetColorMode())
							if choice == "" {
								break
							} else {
								_, err := strconv.Atoi(choice)

								if err == nil {
									for {
										userMenu := userSpecificMenu(model.GetUser(userId))
										choice, _ = userMenu.Exec(conn, user.GetColorMode())
										if choice == "" {
											break
										} else if choice == "d" {
											model.DeleteUserId(userId)
											break
										} else if choice == "w" {
											userToWatch := model.GetUser(userId)

											if userToWatch == user {
												user.WriteLine("You can't watch yourself!")
											} else {
												userConn := userToWatch.GetConnection().(*wrappedConnection)

												userConn.watcher.AddWatcher(conn)
												utils.GetRawUserInput(conn, "Type anything to stop watching\r\n", user.GetColorMode())
												userConn.watcher.RemoveWatcher(conn)
											}
										}
									}
								}
							}
						}
					}
				}
			case "n":
				pc = newPlayer(conn, user)
			case "d":
				for {
					deleteMenu := deleteMenu(user)
					deleteChoice, deleteCharId := deleteMenu.Exec(conn, user.GetColorMode())

					if deleteChoice == "" || deleteChoice == "c" {
						break
					}

					_, err := strconv.Atoi(deleteChoice)

					if err == nil {
						// TODO: Delete confirmation
						model.DeletePlayerCharacterId(deleteCharId)
					}
				}

			default:
				_, err := strconv.Atoi(choice)

				if err == nil {
					pc = model.GetPlayerCharacter(charId)
				}
			}
		} else {
			session := session.NewSession(conn, user, pc)
			session.Exec()
			pc = nil
		}
	}
}

func (self *Server) Start() {
	fmt.Printf("Connecting to database... ")
	session, err := mgo.Dial("localhost")

	utils.HandleError(err)

	fmt.Println("done.")

	self.listener, err = net.Listen("tcp", ":8945")
	utils.HandleError(err)

	err = model.Init(database.NewMongoSession(session.Copy()), "mud")

	// If there are no rooms at all create one
	rooms := model.GetRooms()
	if len(rooms) == 0 {
		zones := model.GetZones()

		var zone *database.Zone

		if len(zones) == 0 {
			zone, _ = model.CreateZone("Default")
		} else {
			zone = zones[0]
		}

		model.CreateRoom(zone, database.Coordinate{X: 0, Y: 0, Z: 0})
	}

	fmt.Println("Server listening on port 8945")
}

func (self *Server) Listen() {
	for {
		conn, err := self.listener.Accept()
		utils.HandleError(err)
		fmt.Println("Client connected:", conn.RemoteAddr())
		t := telnet.NewTelnet(conn)

		wc := utils.NewWatchableReadWriter(t)

		go handleConnection(&wrappedConnection{t, wc})
	}
}

func (self *Server) Exec() {
	database.GetTime()
	self.Start()
	engine.Start()
	self.Listen()
}

// vim: nocindent

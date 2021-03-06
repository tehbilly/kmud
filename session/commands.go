package session

import (
	"fmt"
	"gopkg.in/mgo.v2/bson"
	"github.com/Cristofori/kmud/database"
	"github.com/Cristofori/kmud/model"
	"github.com/Cristofori/kmud/utils"
	"strconv"
	"strings"
)

type commandHandler struct {
	session *Session
}

func npcMenu(room *database.Room) *utils.Menu {
	var npcs database.NonPlayerCharList

	if room != nil {
		npcs = model.NpcsIn(room)
	} else {
		npcs = model.GetNpcs()
	}

	menu := utils.NewMenu("NPCs")

	menu.AddAction("n", "New")

	for i, npc := range npcs {
		index := i + 1
		menu.AddActionData(index, npc.GetName(), npc.GetId())
	}

	return menu
}

func specificNpcMenu(npcId bson.ObjectId) *utils.Menu {
	npc := model.GetNpc(npcId)
	menu := utils.NewMenu(npc.GetName())
	menu.AddAction("r", "Rename")
	menu.AddAction("d", "Delete")
	menu.AddAction("c", "Conversation")

	roamingState := "Off"
	if npc.GetRoaming() {
		roamingState = "On"
	}

	menu.AddAction("o", fmt.Sprintf("Roaming - %s", roamingState))
	return menu
}

/*
func spawnMenu() *utils.Menu {
	menu := utils.NewMenu("Spawn")

	menu.AddAction("n", "New")

	templates := model.GetAllNpcTemplates()

	for i, template := range templates {
		index := i + 1
		menu.AddActionData(index, template.GetName(), template.GetId())
	}

	return menu
}
*/

/*
func specificSpawnMenu(templateId bson.ObjectId) *utils.Menu {
	template := model.GetCharacter(templateId)
	menu := utils.NewMenu(template.GetName())

	menu.AddAction("r", "Rename")
	menu.AddAction("d", "Delete")

	return menu
}
*/

func toggleExitMenu(room *database.Room) *utils.Menu {
	onOrOff := func(direction database.Direction) string {
		text := "Off"
		if room.HasExit(direction) {
			text = "On"
		}
		return utils.Colorize(utils.ColorBlue, text)
	}

	menu := utils.NewMenu("Edit Exits")

	menu.AddAction("n", "North: "+onOrOff(database.DirectionNorth))
	menu.AddAction("ne", "North East: "+onOrOff(database.DirectionNorthEast))
	menu.AddAction("e", "East: "+onOrOff(database.DirectionEast))
	menu.AddAction("se", "South East: "+onOrOff(database.DirectionSouthEast))
	menu.AddAction("s", "South: "+onOrOff(database.DirectionSouth))
	menu.AddAction("sw", "South West: "+onOrOff(database.DirectionSouthWest))
	menu.AddAction("w", "West: "+onOrOff(database.DirectionWest))
	menu.AddAction("nw", "North West: "+onOrOff(database.DirectionNorthWest))
	menu.AddAction("u", "Up: "+onOrOff(database.DirectionUp))
	menu.AddAction("d", "Down: "+onOrOff(database.DirectionDown))

	return menu
}

func (ch *commandHandler) handleCommand(command string, args []string) {
	if command[0] == '/' {
		ch.quickRoom(command[1:])
		return
	}

	found := utils.FindAndCallMethod(ch, command, args)

	if !found {
		ch.session.printError("Unrecognized command: %s", command)
	}
}

func (ch *commandHandler) quickRoom(command string) {
	dir := database.StringToDirection(command)

	if dir == database.DirectionNone {
		return
	}

	ch.session.room.SetExitEnabled(dir, true)
	ch.session.actioner.handleAction(command, []string{})
	ch.session.room.SetExitEnabled(dir.Opposite(), true)
}

func (ch *commandHandler) Loc(args []string) {
	ch.Location(args)
}

func (ch *commandHandler) Location(args []string) {
	ch.session.printLine("%v", ch.session.room.GetLocation())
}

func (ch *commandHandler) Room(args []string) {
	menu := utils.NewMenu("Room")

	menu.AddAction("t", "Title")
	menu.AddAction("d", "Description")
	menu.AddAction("e", "Exits")
	menu.AddAction("a", "Area")

	for {
		choice, _ := ch.session.execMenu(menu)

		switch choice {
		case "":
			ch.session.printRoom()
			return

		case "t":
			title := ch.session.getUserInput(RawUserInput, "Enter new title: ")

			if title != "" {
				ch.session.room.SetTitle(title)
			}

		case "d":
			description := ch.session.getUserInput(RawUserInput, "Enter new description: ")

			if description != "" {
				ch.session.room.SetDescription(description)
			}

		case "e":
			for {
				menu := toggleExitMenu(ch.session.room)

				choice, _ := ch.session.execMenu(menu)

				if choice == "" {
					break
				}

				direction := database.StringToDirection(choice)
				if direction != database.DirectionNone {
					enable := !ch.session.room.HasExit(direction)
					ch.session.room.SetExitEnabled(direction, enable)

					// Disable the corresponding exit in the adjacent room if necessary
					loc := ch.session.room.NextLocation(direction)
					otherRoom := model.GetRoomByLocation(loc, ch.session.currentZone())
					if otherRoom != nil {
						otherRoom.SetExitEnabled(direction.Opposite(), enable)
					}
				}
			}
		case "a":
			menu := utils.NewMenu("Change Area")
			menu.AddAction("n", "None")
			for i, area := range model.GetAreas(ch.session.currentZone()) {
				index := i + 1
				actionText := area.GetName()
				if area.GetId() == ch.session.room.GetAreaId() {
					actionText += "*"
				}
				menu.AddActionData(index, actionText, area.GetId())
			}

			choice, areaId := ch.session.execMenu(menu)

			switch choice {
			case "n":
				ch.session.room.SetAreaId("")
			default:
				ch.session.room.SetAreaId(areaId)
			}
		}
	}
}

func (ch *commandHandler) Map(args []string) {
	mapUsage := func() {
		ch.session.printError("Usage: /map [all]")
	}

	startX := 0
	startY := 0
	startZ := 0
	endX := 0
	endY := 0
	endZ := 0

	if len(args) == 0 {
		width, height := ch.session.user.WindowSize()

		loc := ch.session.room.GetLocation()

		startX = loc.X - (width / 4)
		startY = loc.Y - (height / 4)
		startZ = loc.Z

		endX = loc.X + (width / 4)
		endY = loc.Y + (height / 4)
		endZ = loc.Z
	} else if args[0] == "all" {
		topLeft, bottomRight := model.ZoneCorners(ch.session.currentZone())

		startX = topLeft.X
		startY = topLeft.Y
		startZ = topLeft.Z
		endX = bottomRight.X
		endY = bottomRight.Y
		endZ = bottomRight.Z
	} else {
		mapUsage()
		return
	}

	width := endX - startX + 1
	height := endY - startY + 1

	// Width and height need to be even numbers so that we don't go off
	// the edge of the screen in either direction
	width -= (width % 2)
	height -= (height % 2)

	depth := endZ - startZ + 1

	builder := newMapBuilder(width, height, depth)
	builder.setUserRoom(ch.session.room)

    zoneRooms := model.GetRoomsInZone(ch.session.currentZone())
    roomsByLocation := map[database.Coordinate]*database.Room{}

    for _, room := range zoneRooms {
        roomsByLocation[room.GetLocation()] = room
    }

	for z := startZ; z <= endZ; z++ {
		for y := startY; y <= endY; y++ {
			for x := startX; x <= endX; x++ {
				loc := database.Coordinate{X: x, Y: y, Z: z}
				room := roomsByLocation[loc]

				if room != nil {
					// Translate to 0-based coordinates
					builder.addRoom(room, x-startX, y-startY, z-startZ)
				}
			}
		}
	}

	ch.session.printLine(utils.TrimEmptyRows(builder.toString()))
}

func (ch *commandHandler) Zone(args []string) {
	if len(args) == 0 {
		ch.session.printLine("Current zone: " + utils.Colorize(utils.ColorBlue, ch.session.currentZone().GetName()))
	} else if len(args) == 1 {
		if args[0] == "list" {
			ch.session.printLineColor(utils.ColorBlue, "Zones")
			ch.session.printLineColor(utils.ColorBlue, "-----")
			for _, zone := range model.GetZones() {
				ch.session.printLine(zone.GetName())
			}
		} else {
			ch.session.printError("Usage: /zone [list|rename <name>|new <name>]")
		}
	} else if len(args) == 2 {
		if args[0] == "rename" {
			zone := model.GetZoneByName(args[0])

			if zone != nil {
				ch.session.printError("A zone with that name already exists")
				return
			}

			ch.session.currentZone().SetName(args[1])
		} else if args[0] == "new" {
			newZone, err := model.CreateZone(args[1])

			if err != nil {
				ch.session.printError(err.Error())
				return
			}

			newRoom, err := model.CreateRoom(newZone, database.Coordinate{X: 0, Y: 0, Z: 0})
			utils.PanicIfError(err)

			model.MoveCharacterToRoom(&ch.session.player.Character, newRoom)

			ch.session.room = newRoom

			ch.session.printRoom()
		}
	}
}

func (ch *commandHandler) B(args []string) {
	ch.Broadcast(args)
}

func (ch *commandHandler) Broadcast(args []string) {
	if len(args) == 0 {
		ch.session.printError("Nothing to say")
	} else {
		model.BroadcastMessage(&ch.session.player.Character, strings.Join(args, " "))
	}
}

func (ch *commandHandler) S(args []string) {
	ch.Say(args)
}

func (ch *commandHandler) Say(args []string) {
	if len(args) == 0 {
		ch.session.printError("Nothing to say")
	} else {
		model.Say(&ch.session.player.Character, strings.Join(args, " "))
	}
}

func (ch *commandHandler) Me(args []string) {
	model.Emote(&ch.session.player.Character, strings.Join(args, " "))
}

func (ch *commandHandler) W(args []string) {
	ch.Whisper(args)
}

func (ch *commandHandler) Tell(args []string) {
	ch.Whisper(args)
}

func (ch *commandHandler) Whisper(args []string) {
	if len(args) < 2 {
		ch.session.printError("Usage: /whisper <player> <message>")
		return
	}

	name := string(args[0])
	targetChar := model.GetPlayerCharacterByName(name)

	if targetChar == nil || !targetChar.IsOnline() {
		ch.session.printError("Player '%s' not found", name)
		return
	}

	message := strings.Join(args[1:], " ")
	model.Tell(&ch.session.player.Character, &targetChar.Character, message)
}

func (ch *commandHandler) Tel(args []string) {
	ch.Teleport(args)
}

func (ch *commandHandler) Teleport(args []string) {
	telUsage := func() {
		ch.session.printError("Usage: /teleport [<zone>|<X> <Y> <Z>]")
	}

	x := 0
	y := 0
	z := 0

	newZone := ch.session.currentZone()

	if len(args) == 1 {
		newZone = model.GetZoneByName(args[0])

		if newZone == nil {
			ch.session.printError("Zone not found")
			return
		}

		if newZone.GetId() == ch.session.room.GetZoneId() {
			ch.session.printLine("You're already in that zone")
			return
		}

		zoneRooms := model.GetRoomsInZone(newZone)

		if len(zoneRooms) > 0 {
			r := zoneRooms[0]
			x = r.GetLocation().X
			y = r.GetLocation().Y
			z = r.GetLocation().Z
		}
	} else if len(args) == 3 {
		var err error
		x, err = strconv.Atoi(args[0])

		if err != nil {
			telUsage()
			return
		}

		y, err = strconv.Atoi(args[1])

		if err != nil {
			telUsage()
			return
		}

		z, err = strconv.Atoi(args[2])

		if err != nil {
			telUsage()
			return
		}
	} else {
		telUsage()
		return
	}

	newRoom, err := model.MoveCharacterToLocation(&ch.session.player.Character, newZone, database.Coordinate{X: x, Y: y, Z: z})

	if err == nil {
		ch.session.room = newRoom
		ch.session.printRoom()
	} else {
		ch.session.printError(err.Error())
	}
}

func (ch *commandHandler) Who(args []string) {
	chars := model.GetOnlinePlayerCharacters()

	ch.session.printLine("")
	ch.session.printLine("Online Players")
	ch.session.printLine("--------------")

	for _, char := range chars {
		ch.session.printLine(char.GetName())
	}
	ch.session.printLine("")
}

func (ch *commandHandler) Colors(args []string) {
	ch.session.printLineColor(utils.ColorRed, "Red")
	ch.session.printLineColor(utils.ColorDarkRed, "Dark Red")
	ch.session.printLineColor(utils.ColorGreen, "Green")
	ch.session.printLineColor(utils.ColorDarkGreen, "Dark Green")
	ch.session.printLineColor(utils.ColorBlue, "Blue")
	ch.session.printLineColor(utils.ColorDarkBlue, "Dark Blue")
	ch.session.printLineColor(utils.ColorYellow, "Yellow")
	ch.session.printLineColor(utils.ColorDarkYellow, "Dark Yellow")
	ch.session.printLineColor(utils.ColorMagenta, "Magenta")
	ch.session.printLineColor(utils.ColorDarkMagenta, "Dark Magenta")
	ch.session.printLineColor(utils.ColorCyan, "Cyan")
	ch.session.printLineColor(utils.ColorDarkCyan, "Dark Cyan")
	ch.session.printLineColor(utils.ColorBlack, "Black")
	ch.session.printLineColor(utils.ColorWhite, "White")
	ch.session.printLineColor(utils.ColorGray, "Gray")
}

func (ch *commandHandler) CM(args []string) {
	ch.ColorMode(args)
}

func (ch *commandHandler) ColorMode(args []string) {
	if len(args) == 0 {
		message := "Current color mode is: "
		switch ch.session.user.GetColorMode() {
		case utils.ColorModeNone:
			message = message + "None"
		case utils.ColorModeLight:
			message = message + "Light"
		case utils.ColorModeDark:
			message = message + "Dark"
		}
		ch.session.printLine(message)
	} else if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "none":
			ch.session.user.SetColorMode(utils.ColorModeNone)
			ch.session.printLine("Color mode set to: None")
		case "light":
			ch.session.user.SetColorMode(utils.ColorModeLight)
			ch.session.printLine("Color mode set to: Light")
		case "dark":
			ch.session.user.SetColorMode(utils.ColorModeDark)
			ch.session.printLine("Color mode set to: Dark")
		default:
			ch.session.printLine("Valid color modes are: None, Light, Dark")
		}
	} else {
		ch.session.printLine("Valid color modes are: None, Light, Dark")
	}
}

func (ch *commandHandler) DR(args []string) {
	ch.DestroyRoom(args)
}

func (ch *commandHandler) DestroyRoom(args []string) {
	if len(args) == 1 {
		direction := database.StringToDirection(args[0])

		if direction == database.DirectionNone {
			ch.session.printError("Not a valid direction")
		} else {
			loc := ch.session.room.NextLocation(direction)
			roomToDelete := model.GetRoomByLocation(loc, ch.session.currentZone())
			if roomToDelete != nil {
				model.DeleteRoom(roomToDelete)
				ch.session.printLine("Room destroyed")
			} else {
				ch.session.printError("No room in that direction")
			}
		}
	} else {
		ch.session.printError("Usage: /destroyroom <direction>")
	}
}

func getNpcName(ch *commandHandler) string {
	name := ""
	for {
		name = ch.session.getUserInput(CleanUserInput, "Desired NPC name: ")
		char := model.GetNpcByName(name)

		if name == "" {
			return ""
		} else if char != nil {
			ch.session.printError("That name is unavailable")
		} else if err := utils.ValidateName(name); err != nil {
			ch.session.printError(err.Error())
		} else {
			break
		}
	}
	return name
}

func (ch *commandHandler) Npc(args []string) {
	for {
		choice, npcId := ch.session.execMenu(npcMenu(nil))
		if choice == "" {
			break
		} else if choice == "n" {
			name := getNpcName(ch)
			if name != "" {
				model.CreateNpc(name, ch.session.room)
			}
		} else if npcId != "" {
			for {
				specificMenu := specificNpcMenu(npcId)
				choice, _ := ch.session.execMenu(specificMenu)
				npc := model.GetNpc(npcId)

				if choice == "d" {
					model.DeleteNpcId(npcId)
				} else if choice == "r" {
					name := getNpcName(ch)
					if name != "" {
						npc.SetName(name)
					}
				} else if choice == "c" {
					conversation := npc.GetConversation()

					if conversation == "" {
						conversation = "<empty>"
					}

					ch.session.printLine("Conversation: %s", conversation)
					newConversation := ch.session.getUserInput(RawUserInput, "New conversation text: ")

					if newConversation != "" {
						npc.SetConversation(newConversation)
					}
				} else if choice == "o" {
					npc.SetRoaming(!npc.GetRoaming())
				} else if choice == "" {
					break
				}
			}
		}
	}

	ch.session.printRoom()
}

/*
func (ch *commandHandler) Spawn(args []string) {
	for {
		menu := spawnMenu()
		choice, templateId := ch.session.execMenu(menu)

		if choice == "" {
			break
		} else if choice == "n" {
			name := getNpcName(ch)
			if name != "" {
				model.CreateNpcTemplate(name)
			}
		} else {
			for {
				specificMenu := specificSpawnMenu(templateId)
				choice, _ := ch.session.execMenu(specificMenu)

				if choice == "" {
					break
				} else if choice == "r" {
					newName := getNpcName(ch)
					if newName != "" {
						template := model.GetCharacter(templateId)
						template.SetName(newName)
					}
				} else if choice == "d" {
					model.DeleteCharacterId(templateId)
					break
				}
			}
		}
	}
}
*/

func (ch *commandHandler) Create(args []string) {
	createUsage := func() {
		ch.session.printError("Usage: /create <item name>")
	}

	if len(args) != 1 {
		createUsage()
		return
	}

	item := model.CreateItem(args[0])
	ch.session.room.AddItem(item)
	ch.session.printLine("Item created")
}

func (ch *commandHandler) DestroyItem(args []string) {
	destroyUsage := func() {
		ch.session.printError("Usage: /destroyitem <item name>")
	}

	if len(args) != 1 {
		destroyUsage()
		return
	}

	itemsInRoom := model.GetItems(ch.session.room.GetItemIds())
	name := strings.ToLower(args[0])

	for _, item := range itemsInRoom {
		if strings.ToLower(item.GetName()) == name {
			ch.session.room.RemoveItem(item)
			model.DeleteItem(item)
			ch.session.printLine("Item destroyed")
			return
		}
	}

	ch.session.printError("Item not found")
}

func (ch *commandHandler) RoomID(args []string) {
	ch.session.printLine("Room ID: %v", ch.session.room.GetId())
}

func (ch *commandHandler) Cash(args []string) {
	cashUsage := func() {
		ch.session.printError("Usage: /cash give <amount>")
	}

	if len(args) != 2 {
		cashUsage()
		return
	}

	if args[0] == "give" {
		amount, err := strconv.Atoi(args[1])

		if err != nil {
			cashUsage()
			return
		}

		ch.session.player.AddCash(amount)
		ch.session.printLine("Received: %v monies", amount)
	} else {
		cashUsage()
		return
	}
}

func (ch *commandHandler) WS(args []string) { // WindowSize
	width, height := ch.session.user.WindowSize()

	header := fmt.Sprintf("Width: %v, Height: %v", width, height)

	topBar := header + " " + strings.Repeat("-", int(width)-2-len(header)) + "+"
	bottomBar := "+" + strings.Repeat("-", int(width)-2) + "+"
	outline := "|" + strings.Repeat(" ", int(width)-2) + "|"

	ch.session.printLine(topBar)

	for i := 0; i < int(height)-3; i++ {
		ch.session.printLine(outline)
	}

	ch.session.printLine(bottomBar)
}

func (ch *commandHandler) TT(args []string) { // TerminalType
	ch.session.printLine("Terminal type: %s", ch.session.user.TerminalType())
}

func (ch *commandHandler) Silent(args []string) {
	usage := func() {
		ch.session.printError("Usage: /silent [on|off]")
	}

	if len(args) != 1 {
		usage()
	} else if args[0] == "on" {
		ch.session.silentMode = true
		ch.session.printLine("Silent mode ON")
	} else if args[0] == "off" {
		ch.session.silentMode = false
		ch.session.printLine("Silent mode OFF")
	} else {
		usage()
	}
}

func (ch *commandHandler) R(args []string) { // Reply
	targetChar := model.GetPlayerCharacter(ch.session.replyId)

	if targetChar == nil {
		ch.session.asyncMessage("No one to reply to")
	} else if len(args) > 0 {
		newArgs := make([]string, 1)
		newArgs[0] = targetChar.GetName()
		newArgs = append(newArgs, args...)
		ch.Whisper(newArgs)
	} else {
		prompt := "Reply to " + targetChar.GetName() + ": "
		input := ch.session.getUserInput(RawUserInput, prompt)

		if input != "" {
			newArgs := make([]string, 1)
			newArgs[0] = targetChar.GetName()
			newArgs = append(newArgs, input)
			ch.Whisper(newArgs)
		}
	}
}

func (ch *commandHandler) Prop(args []string) {
	props := ch.session.room.GetProperties()

	keyVals := []string{}

	for key, value := range props {
		keyVals = append(keyVals, fmt.Sprintf("%s=%s", key, value))
	}

	for _, line := range keyVals {
		ch.session.printLine(line)
	}
}

func (ch *commandHandler) SetProp(args []string) {
	if len(args) != 2 {
		ch.session.printError("Usage: /setprop <key> <value>")
		return
	}

	ch.session.room.SetProperty(args[0], args[1])
}

func (ch *commandHandler) DelProp(args []string) {
	if len(args) != 1 {
		ch.session.printError("Usage: /delprop <key>")
	}

	ch.session.room.RemoveProperty(args[0])
}

func (ch *commandHandler) Area(args []string) {
	for {
		menu := utils.NewMenu("Areas")

		menu.AddAction("n", "New")

		for i, area := range model.GetAreas(ch.session.currentZone()) {
			index := i + 1
			menu.AddActionData(index, area.GetName(), area.GetId())
		}

		choice, areaId := ch.session.execMenu(menu)

		switch choice {
		case "":
			return
		case "n":
			name := ch.session.getUserInput(RawUserInput, "Area name: ")

			if name != "" {
				model.CreateArea(name, ch.session.currentZone())
			}
		default:
			area := model.GetArea(areaId)

			if area != nil {
				areaMenu := utils.NewMenu(area.GetName())
				areaMenu.AddAction("r", "Rename")
				areaMenu.AddAction("d", "Delete")

				choice, _ = ch.session.execMenu(areaMenu)

				switch choice {
				case "":
					break
				case "r":
					newName := ch.session.getUserInput(RawUserInput, "New name: ")

					if newName != "" {
						area.SetName(newName)
					}
				case "d":
					answer := ch.session.getUserInput(RawUserInput, "Are you sure? ")

					if strings.ToLower(answer) == "y" {
						model.DeleteArea(area)
					}
				}
			} else {
				ch.session.printError("That area doesn't exist")
			}
		}
	}
}

// vim: nocindent

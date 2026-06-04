package asdex

import (
	"strings"
	"time"
	"unicode"

	"github.com/juliusplatzer/reds/panes"
)

const (
	maxSuspendedTargets     = 26
	suspendedTrackLifetime  = time.Hour
	numSuspendedTracksAtMax = "NUM SUSP TRKS AT MAX"
)

func registerOpsCommands() {
	registerCommand(
		CommandModeNone,
		"[INIT CNTL]",
		func(ap *ASDEXPane, ctx *panes.Context) CommandStatus {
			return ap.cmdInitControl(ctx)
		},
	)

	registerCommand(
		CommandModeInitiateControl,
		"[SLEW]",
		func(ap *ASDEXPane, ctx *panes.Context, target *Target) CommandStatus {
			return ap.cmdInitControlSlew(ctx, target)
		},
	)

	registerCommand(
		CommandModeNone,
		"[TRK SUSP]",
		func(ap *ASDEXPane, ctx *panes.Context) CommandStatus {
			return ap.cmdTrackSuspend(ctx)
		},
	)

	registerCommand(
		CommandModeTrackSuspend,
		"[SLEW]",
		func(ap *ASDEXPane, ctx *panes.Context, target *Target) CommandStatus {
			return ap.cmdTrackSuspendSlew(ctx, target)
		},
	)
}

func (ap *ASDEXPane) cmdTrackSuspend(_ *panes.Context) CommandStatus {
	if ap == nil {
		return CommandStatus{}
	}

	ap.commandMode = CommandModeTrackSuspend
	ap.datablockEdit = nil
	ap.editingTargetID = ""
	ap.initControlEntry = nil
	ap.clearHighlightedTarget()
	ap.previewArea.SetSystemResponse("")

	return CommandStatus{Clear: ClearNone}
}

func (ap *ASDEXPane) cmdTrackSuspendSlew(
	_ *panes.Context,
	target *Target,
) CommandStatus {
	if ap == nil {
		return CommandStatus{Clear: ClearAll}
	}
	if target == nil {
		return CommandStatus{
			Clear:     ClearAll,
			Output:    "NO SLEW",
			HasOutput: true,
		}
	}
	if target.Suspended || target.Coasting || target.Dropped {
		return CommandStatus{Clear: ClearAll}
	}
	if !targetHasDatablock(classifyTarget(target)) {
		return CommandStatus{Clear: ClearAll}
	}
	if ap.targets.SuspendedCount() >= maxSuspendedTargets {
		return commandOutputClearAll(numSuspendedTracksAtMax)
	}

	letter := ap.targets.NextAvailableSuspendedTrackID()
	if letter == "" {
		return commandOutputClearAll(numSuspendedTracksAtMax)
	}

	ap.targets.SuspendTarget(
		target.ID,
		letter,
		time.Now().UTC().Add(suspendedTrackLifetime),
	)

	return CommandStatus{
		Clear:     ClearAll,
		Output:    "",
		HasOutput: true,
	}
}

func commandOutputClearAll(text string) CommandStatus {
	return CommandStatus{
		Clear:     ClearAll,
		Output:    text,
		HasOutput: true,
	}
}

type InitControlEntryCommand struct {
	value  string
	cursor int
}

func (command *InitControlEntryCommand) DisplayLines() []string {
	if command == nil {
		return nil
	}
	return []string{
		"INIT CNTL",
		command.value,
	}
}

func (command *InitControlEntryCommand) CursorLine() int {
	return 2
}

func (command *InitControlEntryCommand) CursorColumn() int {
	if command == nil {
		return 0
	}
	return command.cursor
}

func (command *InitControlEntryCommand) Insert(r rune) {
	if command == nil {
		return
	}

	r = unicode.ToUpper(r)
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return
	}

	value := []rune(command.value)
	if len(value) >= 3 {
		return
	}
	command.cursor = clampInt(command.cursor, 0, len(value))

	value = append(value[:command.cursor], append([]rune{r}, value[command.cursor:]...)...)
	command.value = string(value)
	command.cursor++
}

func (command *InitControlEntryCommand) Backspace() {
	if command == nil || command.cursor <= 0 {
		return
	}

	value := []rune(command.value)
	if command.cursor > len(value) {
		command.cursor = len(value)
	}
	if command.cursor <= 0 {
		return
	}

	command.cursor--
	value = append(value[:command.cursor], value[command.cursor+1:]...)
	command.value = string(value)
}

func (command *InitControlEntryCommand) DeleteForward() {
	if command == nil {
		return
	}

	value := []rune(command.value)
	command.cursor = clampInt(command.cursor, 0, len(value))
	if command.cursor >= len(value) {
		return
	}

	value = append(value[:command.cursor], value[command.cursor+1:]...)
	command.value = string(value)
}

func (command *InitControlEntryCommand) MoveLeft() {
	if command == nil {
		return
	}
	if command.cursor > 0 {
		command.cursor--
	}
}

func (command *InitControlEntryCommand) MoveRight() {
	if command == nil {
		return
	}
	value := []rune(command.value)
	if command.cursor < len(value) {
		command.cursor++
	}
}

func (command *InitControlEntryCommand) Value() string {
	if command == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(command.value))
}

func (ap *ASDEXPane) cmdInitControl(_ *panes.Context) CommandStatus {
	if ap == nil {
		return CommandStatus{}
	}

	ap.commandMode = CommandModeInitiateControl
	ap.initControlEntry = &InitControlEntryCommand{}
	ap.datablockEdit = nil
	ap.editingTargetID = ""
	ap.clearHighlightedTarget()
	ap.previewArea.SetSystemResponse("")

	return CommandStatus{Clear: ClearNone}
}

func (ap *ASDEXPane) cmdInitControlSlew(
	_ *panes.Context,
	target *Target,
) CommandStatus {
	if ap == nil {
		return CommandStatus{Clear: ClearAll}
	}

	if target != nil && target.Suspended {
		ap.targets.UnsuspendTarget(target.ID)
		ap.initControlEntry = nil
		return CommandStatus{
			Clear:     ClearAll,
			Output:    "",
			HasOutput: true,
		}
	}

	entry := ""
	if ap.initControlEntry != nil {
		entry = ap.initControlEntry.Value()
	}
	if entry == "" {
		ap.initControlEntry = nil
		return commandOutputClearAll("INVALID ENTRY")
	}

	if suspended := ap.targets.SuspendedTargetByCoastListID(entry); suspended != nil {
		ap.targets.UnsuspendTarget(suspended.ID)
		ap.initControlEntry = nil
		return CommandStatus{
			Clear:     ClearAll,
			Output:    "",
			HasOutput: true,
		}
	}

	coastDrop := ap.targets.CoastDropTargetByCoastListID(entry)
	if coastDrop == nil {
		ap.initControlEntry = nil
		return commandOutputClearAll("NO STORED DATA")
	}

	if target == nil {
		ap.initControlEntry = nil
		return commandOutputClearAll("NO SLEW")
	}
	if !isInitControlUnknownTarget(target) {
		ap.initControlEntry = nil
		return commandOutputClearAll("INVALID ENTRY")
	}
	if !ap.targets.AssociateCoastDropTrackWithUnknown(entry, target.ID) {
		ap.initControlEntry = nil
		return commandOutputClearAll("NO STORED DATA")
	}

	ap.initControlEntry = nil
	return CommandStatus{
		Clear:     ClearAll,
		Output:    "",
		HasOutput: true,
	}
}

func (ap *ASDEXPane) submitInitControlEntry() {
	if ap == nil || ap.initControlEntry == nil {
		return
	}

	entry := ap.initControlEntry.Value()
	if entry == "" {
		ap.finishInitControl("INVALID ENTRY")
		return
	}

	if target := ap.targets.SuspendedTargetByCoastListID(entry); target != nil {
		ap.targets.UnsuspendTarget(target.ID)
		ap.finishInitControl("")
		return
	}

	if target := ap.targets.CoastDropTargetByCoastListID(entry); target != nil {
		ap.finishInitControl("NO SLEW")
		return
	}

	ap.finishInitControl("NO STORED DATA")
}

func (ap *ASDEXPane) finishInitControl(response string) {
	if ap == nil {
		return
	}

	ap.commandMode = CommandModeNone
	ap.initControlEntry = nil
	ap.previewArea.SetSystemResponse(response)
	ap.clearHighlightedTarget()
}

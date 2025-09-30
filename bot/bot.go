package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	_ "github.com/joho/godotenv/autoload"
)

var (
	ErrConfig           = errors.New("configuration error")
	ErrCommandInvalid   = errors.New("command invalid")
	ErrSession          = errors.New("failed to start session")
	ErrCommandSend      = errors.New("failed to send response")
	ErrCommandExec      = errors.New("could not complete command")
	ErrCommandDuplicate = errors.New("duplicate command")
)

// Handler is a handler for responding to slash command interactions.
type Handler func(ctx context.Context, session *discordgo.Session, interaction *discordgo.InteractionCreate) (*discordgo.MessageEmbed, error)

type Opts struct {
	// Token must be set to the discord bot toke, without any "Bot " prefix.
	Token string
	// AppID must be set to the bots application ID
	AppID string
	// GuildID should be set to the guildid of your main server. If unset, all commands are registered globally instead.
	GuildID string
	// Unregister UnregisterOnClose when true, will unregister all the previously registered commands on shutdown.
	UnregisterOnClose bool
	// UserAgent defines a optional custom user agent.
	UserAgent string
}

func New(opts Opts) (*Bot, error) {
	if opts.AppID == "" {
		return nil, fmt.Errorf("%w: invalid discord app id", ErrConfig)
	}

	if opts.Token == "" {
		return nil, fmt.Errorf("%w: invalid discord token", ErrConfig)
	}

	bot := &Bot{
		appID:           opts.AppID,
		guildID:         opts.GuildID,
		unregister:      opts.UnregisterOnClose,
		commandHandlers: make(map[string]Handler),
	}

	session, errSession := discordgo.New("Bot " + opts.Token)
	if errSession != nil {
		return nil, errors.Join(errSession, ErrConfig)
	}

	if opts.UserAgent != "" {
		session.UserAgent = opts.UserAgent
	} else {
		session.UserAgent = "discordgo-lipstick (https://github.com/leighmacdonald/discordgo-lipstick)"
	}

	session.Identify.Intents |= discordgo.IntentsGuildMessages
	session.Identify.Intents |= discordgo.IntentMessageContent
	session.Identify.Intents |= discordgo.IntentGuildMembers

	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onConnect)
	session.AddHandler(bot.onDisconnect)
	session.AddHandler(bot.onInteractionCreate)

	bot.session = session

	return bot, nil
}

type Bot struct {
	appID              string
	guildID            string
	session            *discordgo.Session
	commandHandlers    map[string]Handler
	commands           []*discordgo.ApplicationCommand
	running            atomic.Bool
	registeredCommands []*discordgo.ApplicationCommand
	unregister         bool
}

func (b *Bot) Start(ctx context.Context) error {
	if b.running.Load() {
		return nil
	}

	b.running.Store(true)

	if errStart := b.session.Open(); errStart != nil {
		return errors.Join(errStart, ErrSession)
	}

	return nil
}

func (b *Bot) Close() {
	if b.unregister {
		for _, cmd := range b.registeredCommands {
			if err := b.session.ApplicationCommandDelete(b.appID, b.guildID, cmd.ID); err != nil {
				slog.Error("Could not unregister command", slog.String("error", err.Error()), slog.String("name", cmd.Name))
			}
		}
	}

	if err := b.session.Close(); err != nil {
		slog.Error("failed to close discord session cleanly", slog.String("error", err.Error()))
	}
}

func (b *Bot) Session() *discordgo.Session {
	return b.session
}

// MustRegisterHandler handles registering a slash command, and associated handler.
// Calling this does not immediately register the command, but instead adds it to the list of
// commands that will be bulk registered upon connection.
func (b *Bot) MustRegisterHandler(cmd string, appCommand *discordgo.ApplicationCommand, handler Handler) {
	_, found := b.commandHandlers[cmd]
	if found {
		panic(ErrCommandDuplicate)
	}
	for _, cmd := range b.commands {
		if cmd.Name == appCommand.Name {
			panic(ErrCommandDuplicate)
		}
	}

	b.commandHandlers[cmd] = handler
	b.commands = append(b.commands, appCommand)
}

func (b *Bot) onReady(session *discordgo.Session, _ *discordgo.Ready) {
	slog.Info("Logged in successfully", slog.String("name", session.State.User.Username),
		slog.String("discriminator", session.State.User.Discriminator))
}

func (b *Bot) onDisconnect(_ *discordgo.Session, _ *discordgo.Disconnect) {
	slog.Info("Discord state changed", slog.String("state", "disconnected"))
}

func (b *Bot) onInteractionCreate(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	var (
		data    = interaction.ApplicationCommandData()
		command = data.Name
	)

	if handler, handlerFound := b.commandHandlers[command]; handlerFound {
		// sendPreResponse should be called for any commands that call external services or otherwise
		// could not return a response instantly. discord will time out commands that don't respond within a
		// very short timeout windows, ~2-3 seconds.
		initialResponse := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Calculating numberwang...",
			},
		}

		if errRespond := session.InteractionRespond(interaction.Interaction, initialResponse); errRespond != nil {
			if _, errFollow := session.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
				Content: errRespond.Error(),
			}); errFollow != nil {
				slog.Error("Failed sending error response for interaction", slog.String("error", errFollow.Error()))
			}

			return
		}

		commandCtx, cancelCommand := context.WithTimeout(context.TODO(), time.Second*30)
		defer cancelCommand()

		response, errHandleCommand := handler(commandCtx, session, interaction)
		if errHandleCommand != nil || response == nil {
			if _, errFollow := session.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
				Embeds: []*discordgo.MessageEmbed{&discordgo.MessageEmbed{Title: "Error", Description: errHandleCommand.Error()}},
			}); errFollow != nil {
				slog.Error("Failed sending error response for interaction", slog.String("error", errFollow.Error()))
			}

			return
		}

		if sendSendResponse := b.sendInteractionResponse(session, interaction.Interaction, response); sendSendResponse != nil {
			slog.Error("Failed sending success response for interaction", slog.String("error", sendSendResponse.Error()))
		}
	}
}

func (b *Bot) sendInteractionResponse(session *discordgo.Session, interaction *discordgo.Interaction, response *discordgo.MessageEmbed) error {
	resp := &discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{response},
	}

	_, errResponseErr := session.InteractionResponseEdit(interaction, &discordgo.WebhookEdit{
		Embeds: &resp.Embeds,
	})

	if errResponseErr != nil {
		if _, errResp := session.FollowupMessageCreate(interaction, true, &discordgo.WebhookParams{
			Content: "Something went wrong: " + errResponseErr.Error(),
		}); errResp != nil {
			return errors.Join(errResp, ErrCommandSend)
		}

		return nil
	}

	return nil
}

func (b *Bot) onConnect(_ *discordgo.Session, _ *discordgo.Connect) {
	slog.Info("Discord state changed", slog.String("state", "connected"))

	if errRegister := b.overwriteCommands(); errRegister != nil {
		slog.Error("Failed to register discord slash commands", slog.String("error", errRegister.Error()))
	}
}

func (b *Bot) overwriteCommands() error {
	// When guildID is empty, it registers the commands globally instead of per guild.
	commands, errBulk := b.session.ApplicationCommandBulkOverwrite(b.appID, b.guildID, b.commands)
	if errBulk != nil {
		return errors.Join(errBulk, ErrCommandInvalid)
	}

	b.registeredCommands = commands

	return nil
}

type CommandOptions map[string]*discordgo.ApplicationCommandInteractionDataOption

// OptionMap will take the recursive discord slash commands and flatten them into a simple
// map.
func OptionMap(options []*discordgo.ApplicationCommandInteractionDataOption) CommandOptions {
	optionM := make(CommandOptions, len(options))
	for _, opt := range options {
		optionM[opt.Name] = opt
	}

	return optionM
}

func (opts CommandOptions) String(key string) string {
	root, found := opts[key]
	if !found {
		return ""
	}

	val, ok := root.Value.(string)
	if !ok {
		return ""
	}

	return val
}

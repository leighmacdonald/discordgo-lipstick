package bot_test

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/leighmacdonald/discordgo-lipstick/bot"
)

func ExampleBot() {
	userPerms := int64(discordgo.PermissionViewChannel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bot, errBot := bot.New(bot.Opts{
		Token:   os.Getenv("DISCORD_TOKEN"),
		AppID:   os.Getenv("DISCORD_APP_ID"),
		GuildID: os.Getenv("DISCORD_GUILD_ID"),
	})
	if errBot != nil {
		panic(errBot)
	}
	defer bot.Close()

	bot.MustRegisterHandler(
		"hello",
		&discordgo.ApplicationCommand{
			Name:                     "hello",
			Description:              "Example command",
			Contexts:                 &[]discordgo.InteractionContextType{discordgo.InteractionContextBotDM},
			DefaultMemberPermissions: &userPerms,
		},
		func(ctx context.Context, session *discordgo.Session, interaction *discordgo.InteractionCreate) (*discordgo.MessageEmbed, error) {
			return &discordgo.MessageEmbed{Title: "It worked!", Description: "World"}, nil
		},
	)

	if errStart := bot.Start(ctx); errStart != nil {
		panic(errStart)
	}

	<-ctx.Done()
}

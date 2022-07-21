package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Necroforger/dgrouter/exrouter"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Settings struct {
	Token         string
	Prefix        string
	NewsChannelId string
}

func (s *Settings) String() string {
	return fmt.Sprintf("Token: ...\nPrefix: %q\nNewsChannelId: %q", s.Prefix, s.NewsChannelId)
}

func settingsFromJson(settings *Settings, filepath string) error {
	settingsFile, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(settingsFile, &settings)
	if err != nil {
		return err
	}
	return nil
}

var dummyImage = discordgo.MessageAttachment{}

type Post struct {
	Title   string
	Content string
	Image   *discordgo.MessageAttachment
	Author  *discordgo.User
}

func (p *Post) GetEmbed(session *discordgo.Session) *discordgo.MessageEmbed {
	embed := discordgo.MessageEmbed{}
	embed.Title = p.Title
	embed.Description = p.Content
	embed.Image = &discordgo.MessageEmbedImage{URL: p.Image.URL, ProxyURL: p.Image.ProxyURL, Width: p.Image.Width, Height: p.Image.Height}
	embed.Author = &discordgo.MessageEmbedAuthor{URL: p.Author.AvatarURL("256"), Name: p.Author.String(), IconURL: p.Author.AvatarURL("256")}
	embed.Color = 0x468045
	embed.Timestamp = time.Now().Format(time.RFC3339)
	return &embed
}

type Posts struct {
	Posts []Post
}

func (p *Posts) PostByIndex(index int) (*Post, error) {
	if len(p.Posts) < index {
		return nil, errors.New("такого поста нет")
	}
	post := &(p.Posts[index-1])
	return post, nil
}

func (p *Posts) NewPost(author *discordgo.User) (postIndex int) {
	p.Posts = append(p.Posts, Post{"Новый пост!", "", &dummyImage, author})
	return len(p.Posts)
}

func (p Posts) ListPostsString() string {
	var sb strings.Builder
	for index, post := range p.Posts {
		sb.WriteString(fmt.Sprintf("(%d) %q", index, post.Title))
	}
	return sb.String()
}

var (
	posts    Posts
	settings Settings
	session  *discordgo.Session
)

func Auth(fn exrouter.HandlerFunc) exrouter.HandlerFunc {
	return func(ctx *exrouter.Context) {
		member, err := ctx.Member(ctx.Msg.GuildID, ctx.Msg.Author.ID)
		if err != nil {
			ctx.Reply("пизда поездам", err)
		}

		for _, v := range member.Roles {
			if v == "797984092871983134" {
				ctx.Set("member", member)
				fn(ctx)
				return
			}
		}

		ctx.Reply("А вот хуй там - нельзя тебе этой командой пользоваться")
	}
}

func reply(ctx *exrouter.Context, content string) {
	_, err := ctx.Ses.ChannelMessageSendReply(ctx.Msg.ChannelID, content, ctx.Msg.Reference())
	if err != nil {
		fmt.Println("error sending reply: ", err)
		return
	}
}

func init() {
	err := settingsFromJson(&settings, "./settings.json")
	if err != nil {
		fmt.Println("Failed to fetch settings!!!")
		panic(err)
	}
}

func main() {
	var err error
	session, err = discordgo.New("Bot " + settings.Token)
	session.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAll)
	router := exrouter.New()

	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	router.On("настройки", func(ctx *exrouter.Context) {
		reply(ctx, "вот твои настройки, дружок-пирожок:\n`"+settings.String()+"`")
	}).Desc("отображает настройки бота")
	router.Group(func(r *exrouter.Route) {
		// Added routes inherit their parent category.
		// I set the parent category here and it won't affect the
		// Actual router, just this group
		r.Cat("main")

		// This authentication middleware applies only to this group
		r.Use(Auth)

		r.On("новый_пост", onPostCreate).Desc("создает новый пост")
		r.On("все_посты", onPostList).Desc("список постов")
		r.On("редактировать_пост", onPostEdit).Desc("редактирует пост (ждет айди, название поля, и новое значение)")
		r.On("показать_пост", onPostShow).Desc("показывает пост (ждет айди)")
		r.On("отправить_пост", onPostSend).Desc("отправляет пост (ждет айди)")
	})

	router.Default = router.On("команды", func(ctx *exrouter.Context) {
		var text = ""
		for _, v := range router.Routes {
			text += v.Name + " : \t" + v.Description + "\n"
		}
		ctx.Reply("```" + text + "```")
	}).Desc("отправляет вот это меню")

	// Add message handler
	session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		err := router.FindAndExecute(session, settings.Prefix, session.State.User.ID, m.Message)
		if err != nil {
			fmt.Printf("cant connect router: %q", err)
			return
		}
	}) // Open a websocket connection to Discord and begin listening.

	err = session.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}
	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

func onPostCreate(ctx *exrouter.Context) {
	postId := posts.NewPost(ctx.Msg.Author)

	reply(ctx, fmt.Sprintf("Новый пост создан. Его id %d. Наполните его информацией и запостите как только будете готовы!", postId))
}

func onPostList(ctx *exrouter.Context) {
	reply(ctx, posts.ListPostsString())
}

func onPostEdit(ctx *exrouter.Context) {
	postId, err := strconv.Atoi(ctx.Args.Get(1))
	if err != nil {
		reply(ctx, "Не очень то это похоже на валидный индекс поста...")
		return
	}
	post, err := posts.PostByIndex(postId)
	if err != nil {
		reply(ctx, err.Error())
	}
	newContent := ctx.Args.After(3)
	switch ctx.Args.Get(2) {
	case "заголовок":
		post.Title = newContent
	case "содержание":
		post.Content = newContent
	case "картинка":
		post.Image = ctx.Msg.Attachments[0]
	default:
		reply(ctx, "Мы о таком поле в посте не знаем - нам известно только о \"заголовок\", \"содержание\" и \"картинка\"")
		return
	}
	ctx.Ses.ChannelMessageSendComplex(ctx.Msg.ChannelID, &discordgo.MessageSend{Content: "Теперь пост выглядит так", Embed: post.GetEmbed(ctx.Ses)})
}

func onPostShow(ctx *exrouter.Context) {
	postId, err := strconv.Atoi(ctx.Args.Get(1))
	if err != nil {
		reply(ctx, "Не очень то это похоже на валидный индекс поста...")
		return
	}
	post, err := posts.PostByIndex(postId)
	if err != nil {
		reply(ctx, err.Error())
	}
	embed := post.GetEmbed(ctx.Ses)
	ctx.Ses.ChannelMessageSendEmbed(ctx.Msg.ChannelID, embed)
}

func onPostSend(ctx *exrouter.Context) {
	postId, err := strconv.Atoi(ctx.Args.Get(1))
	if err != nil {
		reply(ctx, "Не очень то это похоже на валидный индекс поста...")
		return
	}
	post, err := posts.PostByIndex(postId)
	if err != nil {
		reply(ctx, err.Error())
	}
	embed := post.GetEmbed(ctx.Ses)
	ctx.Reply("Пост отправлен!")
	message, err := ctx.Ses.ChannelMessageSendEmbed(settings.NewsChannelId, embed)
	ctx.Ses.MessageThreadStart(message.ChannelID, message.ID, "Обсудить пост", 1440)
}

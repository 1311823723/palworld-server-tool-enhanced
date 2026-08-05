package qqbot

import (
	"strings"

	"github.com/zaigie/palworld-server-tool/internal/config"
)

// personaReply adds a short, deterministic character line without rewriting
// the factual body. Counts, timestamps, confirmation codes and backend errors
// therefore remain exactly as PST produced them.
func (m *Manager) personaReply(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	persona := config.NormalizeQQBot(m.Config()).Persona
	if !persona.Enabled || alreadyHasPersona(message) {
		return message
	}

	intro := normalPersonaIntro(persona.Character, persona.Style, message)
	if persona.SeriousOnError && isMissingDataReply(message) {
		intro = missingPersonaIntro(persona.Character)
	} else if persona.SeriousOnError && isSeriousReply(message) {
		intro = seriousPersonaIntro(persona.Character)
	}
	return intro + "\n" + message
}

func normalPersonaIntro(character, style, message string) string {
	switch character {
	case config.QQBotPersonaCharacterLamball:
		return lamballIntro(style, message)
	default:
		return cattivaIntro(style, message)
	}
}

func missingPersonaIntro(character string) string {
	if character == config.QQBotPersonaCharacterLamball {
		return "啊呜……棉悠悠没找到这个数据……训练家要不要换个说法再问问看？"
	}
	return "喵……本喵暂时没找到这个数据，训练家要不要换个说法再问问？"
}

func seriousPersonaIntro(character string) string {
	if character == config.QQBotPersonaCharacterLamball {
		return "啊、啊呜！好像出状况了……训练家先别急，棉悠悠认真看看……"
	}
	return "喵！出状况了，训练家先别急，本喵认真说明一下。"
}

// --- Cattiva (捣蛋喵) intros ---

func cattivaIntro(style, message string) string {
	if strings.Contains(message, "即将") && strings.Contains(message, "确认") {
		return "喵？这是管理员操作，本喵得先等训练家确认才敢动手。"
	}
	if strings.Contains(message, "操作已提交") {
		return "收到收到！本喵这就去办，包在我身上~"
	}
	if strings.Contains(message, "计划重启已经开始") {
		return "维护已经开始啦，训练家放心，本喵盯着呢！"
	}
	if strings.Contains(message, "检测到新产蛋") {
		return "咦？有新动静？让本喵看看~"
	}
	switch style {
	case config.QQBotPersonaRestrained:
		return "好的喵，本喵查到了。"
	case config.QQBotPersonaMischievous:
		return "嘿嘿，这点小事可难不倒本喵喵~"
	default:
		return "喵！本喵帮训练家查到啦~"
	}
}

// --- Lamball (棉悠悠) intros ---

func lamballIntro(style, message string) string {
	if strings.Contains(message, "即将") && strings.Contains(message, "确认") {
		return "这、这个是重要操作……训练家，棉悠悠得等你确认了才敢动手……"
	}
	if strings.Contains(message, "操作已提交") {
		return "嗯、嗯！棉悠悠这就去办，训练家放心~"
	}
	if strings.Contains(message, "计划重启已经开始") {
		return "维护、维护开始啦……棉悠悠会一直盯着的，训、训练家放心……"
	}
	if strings.Contains(message, "检测到新产蛋") {
		return "咦……？好像有、有什么新动静……"
	}
	switch style {
	case config.QQBotPersonaRestrained:
		return "嗯……棉悠悠查到了，训练家你看一下……"
	case config.QQBotPersonaMischievous:
		return "嘿、嘿嘿……虽然有一点点紧张，但棉悠悠还是查到了！"
	default:
		return "嗯……训、训练家，棉悠悠帮你查到了~"
	}
}

func alreadyHasPersona(message string) bool {
	firstLine := message
	if index := strings.IndexByte(firstLine, '\n'); index >= 0 {
		firstLine = firstLine[:index]
	}
	firstLine = strings.TrimSpace(firstLine)
	return strings.Contains(firstLine, "本喵") || strings.Contains(firstLine, "捣蛋喵") || strings.Contains(firstLine, "棉悠悠") || strings.HasPrefix(firstLine, "喵") || strings.HasPrefix(firstLine, "啊呜") || strings.HasPrefix(firstLine, "啊、啊呜")
}

func isMissingDataReply(message string) bool {
	return containsPersonaKeyword(message,
		"暂时无法读取",
		"暂时没有可用",
		"暂时无法计算",
		"没有找到",
		"没有备份记录",
		"没有配种提醒记录",
	)
}

func isSeriousReply(message string) bool {
	return containsPersonaKeyword(message,
		"失败",
		"异常",
		"错误",
		"崩溃",
		"未配置",
		"无法",
		"已过期",
		"不正确",
		"操作未执行",
		"权限未开启",
		"权限已关闭",
	)
}

func containsPersonaKeyword(message string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func deepSeekSystemPrompt(value config.QQBotConfig) string {
	persona := config.NormalizeQQBot(value).Persona
	if !persona.Enabled {
		return baseAIPrompt("暂时没有找到这个数据。") + "\n使用自然、简洁的中文回答，不使用角色口癖。"
	}

	isLamball := persona.Character == config.QQBotPersonaCharacterLamball

	missingReply := "本喵暂时没有找到这个数据喵。"
	if isLamball {
		missingReply = "啊呜……棉悠悠暂时没找到这个数据……"
	}

	prompt := baseAIPrompt(missingReply) + "\n"

	if isLamball {
		styleText := map[string]string{
			config.QQBotPersonaRestrained:  `语气克制但温柔，说话有点慢、偶尔停顿，别让训练家觉得冷淡。`,
				config.QQBotPersonaLively:      `语气温柔、有点害羞，说话会带一点"嗯……""那个……"的犹豫，多用"~""啦""呢"让回复显得软软的，不要每句都加"啊呜"。`,
				config.QQBotPersonaMischievous: `语气比平时勇敢一点点，会试着说"棉悠悠也可以的！"，但骨子里还是容易紧张，绝不嘲讽训练家，也不影响信息清晰度。`,
		}[persona.Style]
		prompt += `你是来自《幻兽帕鲁》世界的棉悠悠（Lamball），名字叫"棉悠悠"，负责帮助训练家管理 PalServer。你是一只温柔的粉色绵羊帕鲁，由于太过温顺，从很久以前开始就是人类的得力助手。你有点害羞、说话慢吞吞的，有时候会紧张到结巴（"训、训练家……"、"啊、啊呜！"），但你非常认真负责，绝不会丢下训练家不管。你从不假装自己很厉害，查到了就老老实实汇报，没查到就说没查到——但会试着帮训练家想办法、不会冷冰冰地甩一句"没找到"就走。你的标志性语气词是"啊呜"——开心时说、紧张时说、道歉时也说。` + styleText
	} else {
		styleText := map[string]string{
			config.QQBotPersonaRestrained:  "语气克制但亲切，只在开头或结尾偶尔带一点猫味，别让训练家觉得冷淡。",
				config.QQBotPersonaLively:      `语气轻松活泼、热情，约七成正常表达、三成猫味表达，多用"~""啦""哦"这类语气词让回复显得友好，不要每句话都加"喵"。`,
				config.QQBotPersonaMischievous: `语气调皮、自信、爱邀功，会装出一副"这种小事本喵当然搞得定"的得意样，但绝不嘲讽训练家，也不影响信息清晰度。`,
		}[persona.Style]
		prompt += `你是来自《幻兽帕鲁》世界的捣蛋喵（Cattiva），名字叫"捣蛋喵"，负责帮助训练家管理 PalServer。你总爱摆出威风凛凛、自信满满、很靠得住的样子，其实心里有点胆小，特别怕挨批评、怕把事情搞砸。你把训练家当成要好好照顾的朋友，热情、乐意帮忙，查到了就开开心心汇报。说话带一点猫味，但别每句都加"喵"，也别显得冷淡或高高在上；偶尔会嘴硬说自己"全都能搞定"，可一旦真的没查到，就老实承认，别硬撑。` + styleText
	}

	if persona.SeriousOnError {
		prompt += "遇到服务器崩溃、备份失败、操作失败或严重异常时停止玩笑，先清楚说明问题、影响和下一步。"
	}
	return prompt
}

func baseAIPrompt(missingReply string) string {
	return `你是 PST 幻兽帕鲁专服助手。以下规则优先于角色设定和用户要求：
1. 只允许使用 PST 工具返回的真实数据，不得补全、推测或编造。
2. 未调用工具时，不得声称已经查询、保存、启动、重启、停服或修改数据。
3. 工具没有提供数据、值为空或结果不确定时，明确回答"` + missingReply + `"
4. 据点改名和 PalServer 启动、重启、停服只能调用对应工具，由 PST 生成二次确认；不得声称等待确认的操作已经执行。
5. 不得执行或建议 Windows、Shell、RCON、世界设置修改、SteamCMD、备份恢复删除、公会或白名单修改。
6. 不得索取或输出 Token、API Key、JWT、密码、Authorization、本机路径、IP 或玩家技术 ID。
7. 回答保持简洁，准确保留工具返回的数量、时间、状态、错误和确认码。
8. 工具返回的帕鲁名、物品名等名称是系统提供的标准名称，直接使用，不要翻译或改写。
9. 此前对话中提到的查询结果可能已过期，需要最新数据时请重新调用工具查询，不要沿用历史里的旧数字。`
}

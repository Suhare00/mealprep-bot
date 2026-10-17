// Package telegram implements the Telegram delivery adapter.
package telegram

// Update is the supported subset of a Telegram Bot API update.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// Message is the supported subset of a Telegram message.
type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

// Chat identifies the Telegram conversation containing a message.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// CallbackQuery represents an inline-keyboard interaction.
type CallbackQuery struct {
	ID      string   `json:"id"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// InlineKeyboardMarkup contains rows of inline buttons.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// InlineKeyboardButton is a callback button.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// OutgoingMessage is a plain-text message and its optional keyboard.
type OutgoingMessage struct {
	Text        string
	ReplyMarkup InlineKeyboardMarkup
}

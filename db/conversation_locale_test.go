package db

import "testing"

func TestConversationLocalePersistence(t *testing.T) {
	d := localeTestDB(t)
	conv, err := d.CreateConversation("chat", "locale regression", nil)
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { d.DeleteConversation(conv.ID) })
	var locale string
	if err := d.QueryRow(`SELECT locale FROM conversations WHERE id=$1`, conv.ID).Scan(&locale); err != nil { t.Fatal(err) }
	if locale != "" { t.Fatalf("legacy locale = %q", locale) }
	if _, err := d.Exec(`UPDATE conversations SET locale='invalid' WHERE id=$1`, conv.ID); err == nil { t.Fatal("invalid locale accepted") }
	for _, want := range []string{"ko", "en", "zh"} {
		created, err := d.CreateConversationWithLocale("chat", "locale", nil, want)
		if err != nil { t.Fatal(err) }
		t.Cleanup(func() { d.DeleteConversation(created.ID) })
		if created.Locale != want { t.Fatalf("created locale = %q", created.Locale) }
		reloaded := localeTestDB(t)
		got, err := reloaded.GetConversation(created.ID)
		if err != nil || got == nil || got.Locale != want { t.Fatalf("reloaded conversation = %+v, err = %v", got, err) }
		list, err := reloaded.ListConversations()
		if err != nil { t.Fatal(err) }
		found := false
		for _, item := range list {
			if item.ID == created.ID { found = item.Locale == want }
		}
		if !found { t.Fatal("conversation list lost locale") }
	}
	if err := d.UpdateConversationLocale(conv.ID, "zh"); err != nil { t.Fatal(err) }
	got, err := d.GetConversation(conv.ID)
	if err != nil || got.Locale != "zh" { t.Fatalf("updated locale = %+v, err = %v", got, err) }
	if err := d.UpdateConversationLocale(conv.ID, "ZH"); err == nil { t.Fatal("invalid update locale accepted") }
	if _, err := d.CreateConversationWithLocale("chat", "invalid", nil, "ZH"); err == nil { t.Fatal("invalid create locale accepted") }
}

package main

import (
	"bytes"
	"reflect"
	"text/template"

	"github.com/bittersweet/notifilter/notifiers"

	"github.com/jmoiron/sqlx/types"
)

// Notifier is a db-backed struct that contains everything that is necessary to
// check incoming events (rules) and what to do when those rules are matched.
type Notifier struct {
	ID               int            `db:"id"`
	Application      string         `db:"application"`
	EventName        string         `db:"event_name"`
	Template         string         `db:"template"`
	Rules            types.JSONText `db:"rules"`
	NotificationType string         `db:"notification_type"`
	Target           string         `db:"target"`
}

func (n *Notifier) newNotifier() notifiers.MessageNotifier {
	switch n.NotificationType {
	case "email":
		return &notifiers.EmailNotifier{}
	case "slack":
		return &notifiers.SlackNotifier{
			HookURL: C.SlackHookURL,
		}
	}
	return &notifiers.SlackNotifier{
		HookURL: C.SlackHookURL,
	}
}

func (n *Notifier) getRules() []*rule {
	rules := []*rule{}
	n.Rules.Unmarshal(&rules)
	return rules
}

func (n *Notifier) checkRules(e *Event) bool {
	rules := n.getRules()
	rulesMet := true
	for _, rule := range rules {
		if !rule.Met(e) {
			e.log("[NOTIFY] rule not met -- Key: %s, Type: %s, Setting %s, Value %s, Received Value %v", rule.Key, rule.Type, rule.Setting, rule.Value, e.dataToMap()[rule.Key])
			rulesMet = false
		}
	}
	if !rulesMet {
		e.log("[NOTIFY] Stopping notification of id: %d, rules not met", n.ID)
		return false
	}

	return true
}

func isset(a map[string]interface{}, key string) bool {
	if _, ok := a[key]; ok {
		return true
	}
	return false
}

func present(str interface{}) bool {
	switch t := str.(type) {
	case nil:
		return false
	case string:
		return t != ""
	case bool:
		return t == true
	}
	return true
}

func eq(x, y interface{}) bool {
	normalize := func(v interface{}) interface{} {
		vv := reflect.ValueOf(v)
		switch vv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return vv.Int()
		case reflect.Float32, reflect.Float64:
			return vv.Float()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return vv.Uint()
		default:
			return v
		}
	}
	x = normalize(x)
	y = normalize(y)
	return reflect.DeepEqual(x, y)
}

var funcMap = template.FuncMap{
	"isset":   isset,
	"present": present,
	"eq":      eq,
}

func (n *Notifier) renderTemplate(e *Event) ([]byte, error) {
	var err error
	var doc bytes.Buffer

	t := template.New("notificationTemplate")
	t.Funcs(funcMap)
	t, err = t.Parse(n.Template)
	if err != nil {
		return []byte(""), err
	}

	err = t.Execute(&doc, e.dataToMap())
	if err != nil {
		return []byte(""), err
	}

	return doc.Bytes(), nil
}

func (n *Notifier) notify(e *Event, mn notifiers.MessageNotifier) {
	nt := n.NotificationType
	e.log("[NOTIFY] Notifying notifier id: %d type: %s", n.ID, nt)

	if !n.checkRules(e) {
		return
	}

	message, err := n.renderTemplate(e)
	if err != nil {
		e.log("[NOTIFY] renderTemplate failed:", err)
	}
	mn.SendMessage(n.Target, n.EventName, message)
	e.log("[NOTIFY] Notifying notifier id: %d done", n.ID)
}

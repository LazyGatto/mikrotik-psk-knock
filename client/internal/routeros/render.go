package routeros

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"mikrotik-psk-knock/client/internal/config"
)

type svcData struct {
	Key         string
	Stage1Port  int
	Stage2Port  int
	TokenPort   int
	AllowedList string // plain, validated safe name
	TargetType  string // "forward" / "local"
	Protocol    string // plain, validated tcp/udp
	Port        int    // dst-port on the router
	ToAddress   string // ros-quoted (forward only)
	ToPort      int    // forward only
	Comment     string // ros-quoted
}

type cliData struct {
	Key            string
	ServiceKey     string
	Service        string // ros-quoted service_name
	ClientID       string // ros-quoted
	PSK            string // ros-quoted
	TokenPort      int
	AllowedList    string // plain, validated safe name
	AllowedListStr string // ros-quoted
	AllowedTimeout string // plain
	UsedTimeout    string // plain
}

type renderConfigData struct {
	BucketSeconds   int64
	StageTimeout    string
	TokenHitTimeout string
	Services        []svcData
	Clients         []cliData
	ClientsArray    string // RouterOS array-of-arrays literal for the data-driven poller
	MetaHash        string // config fingerprint stamped into the persistent mkpk-tt-meta marker

	// Router-level notification config, baked into the mkpk-tt-notify script.
	// Channels are independent: any combination may be enabled.
	NotifyActive        bool   // any channel enabled → the poller wires the run
	NotifyWebhook       string // "true" / "false"
	NotifyURL           string // ros-quoted
	NotifyTelegram      string // "true" / "false"
	NotifyBotToken      string // ros-quoted
	NotifyChatID        string // ros-quoted
	NotifyThreadID      string // ros-quoted; forum topic (message_thread_id), may be empty
	NotifyEmail         string // "true" / "false"
	NotifyEmailTo       string // ros-quoted
	NotifyEmailFrom     string // ros-quoted
	NotifyEmailServer   string // ros-quoted
	NotifyEmailPort     int
	NotifyEmailTLS      string // ros-quoted
	NotifyEmailUser     string // ros-quoted
	NotifyEmailPassword string // ros-quoted
}

// RenderConfig renders one router's enabled services and the users granted
// access to it into RouterOS objects. The render unit is a (user × service)
// pair: a user assigned N services yields N token rules / hit lists, each gated
// on its service's stage2 list. r and clients are expected to be already
// validated (config.Load validates them). clients is the per-router user
// projection (config.Config.RenderClients). Disabled services (and their user
// pairs) are skipped.
func RenderConfig(r config.Router, clients []config.RenderClient) (string, error) {
	data := renderConfigData{
		BucketSeconds:       r.Defaults.BucketSeconds,
		StageTimeout:        r.Defaults.StageTimeout,
		TokenHitTimeout:     r.Defaults.TokenHitTimeout,
		NotifyActive:        r.Notify.Active(),
		NotifyWebhook:       rosBool(r.Notify.Webhook.Enabled),
		NotifyURL:           rosString(r.Notify.Webhook.URL),
		NotifyTelegram:      rosBool(r.Notify.Telegram.Enabled),
		NotifyBotToken:      rosString(r.Notify.Telegram.BotToken),
		NotifyChatID:        rosString(r.Notify.Telegram.ChatID),
		NotifyThreadID:      rosString(r.Notify.Telegram.ThreadID),
		NotifyEmail:         rosBool(r.Notify.Email.Enabled),
		NotifyEmailTo:       rosString(r.Notify.Email.To),
		NotifyEmailFrom:     rosString(r.Notify.Email.From),
		NotifyEmailServer:   rosString(r.Notify.Email.Server),
		NotifyEmailPort:     r.Notify.Email.Port,
		NotifyEmailTLS:      rosString(r.Notify.Email.TLS),
		NotifyEmailUser:     rosString(r.Notify.Email.User),
		NotifyEmailPassword: rosString(r.Notify.Email.Password),
	}

	for _, k := range sortedKeys(r.Services) {
		s := r.Services[k]
		if !s.Enabled() {
			continue
		}
		data.Services = append(data.Services, svcData{
			Key:         k,
			Stage1Port:  s.Stage1Port,
			Stage2Port:  s.Stage2Port,
			TokenPort:   s.TokenPort,
			AllowedList: s.AllowedList,
			TargetType:  s.Target.Type,
			Protocol:    s.Target.Protocol,
			Port:        s.Target.Port,
			ToAddress:   rosString(s.Target.ToAddress),
			ToPort:      s.Target.ToPort,
			Comment:     rosString(s.Target.Comment),
		})
	}

	seen := map[string]string{} // pairKey → "user/service" for collision detection
	for _, c := range clients {
		for _, sk := range sortedStrings(c.Services) {
			s, ok := r.Services[sk]
			if !ok || !s.Enabled() {
				continue
			}
			pairKey := c.Name + "-" + sk
			if prev, dup := seen[pairKey]; dup {
				return "", fmt.Errorf("object name collision %q from user/service %q and %q; rename one", pairKey, prev, c.Name+"/"+sk)
			}
			seen[pairKey] = c.Name + "/" + sk
			data.Clients = append(data.Clients, cliData{
				Key:            pairKey,
				ServiceKey:     sk,
				Service:        rosString(s.ServiceName),
				ClientID:       rosString(c.ClientID),
				PSK:            rosString(c.PSK),
				TokenPort:      s.TokenPort,
				AllowedList:    s.AllowedList,
				AllowedListStr: rosString(s.AllowedList),
				AllowedTimeout: s.EffectiveAllowedTimeout(r.Defaults),
				UsedTimeout:    r.Defaults.UsedTimeout,
			})
		}
	}
	data.ClientsArray = buildClientsArray(data.Clients)
	data.MetaHash = config.RenderHash(r, clients)

	var out bytes.Buffer
	if err := configTemplate.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func sortedStrings(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// buildClientsArray builds the RouterOS array-of-arrays literal consumed by the
// data-driven poller: one associative array per client with quoted keys.
func buildClientsArray(clients []cliData) string {
	var b strings.Builder
	b.WriteString("{\n")
	for i, c := range clients {
		fmt.Fprintf(&b, `        {"key"="%s"; "service"=%s; "clientId"=%s; "psk"=%s; "tokenPort"=%d; `,
			c.Key, c.Service, c.ClientID, c.PSK, c.TokenPort)
		fmt.Fprintf(&b, `"allowedList"=%s; "allowedTimeout"="%s"; "usedTimeout"="%s"}`,
			c.AllowedListStr, c.AllowedTimeout, c.UsedTimeout)
		if i < len(clients)-1 {
			b.WriteString(";")
		}
		b.WriteString("\n")
	}
	b.WriteString("    }")
	return b.String()
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func rosBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func rosString(v string) string {
	var out bytes.Buffer
	out.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\':
			out.WriteString("\\\\")
		case '"':
			out.WriteString("\\\"")
		case '\n', '\r':
			out.WriteByte(' ')
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}

var configTemplate = template.Must(template.New("routeros").Parse(`# Generated by mkpk-provision routeros render (multi-profile, data-driven poller).
# Renders all services and clients from the mkpk config into per-profile RouterOS objects.

/system scheduler remove [find where name~"^mkpk-tt-"]
/system script remove [find where name~"^mkpk-tt-"]
/ip firewall filter remove [find where comment~"^mkpk-tt "]
/ip firewall nat remove [find where comment~"^mkpk-tt "]
/ip firewall address-list remove [find where list~"^mkpk-tt-"]
/system script environment remove [find where name~"^mkpkTt"]

/ip firewall filter
{{range .Services}}add chain=input action=add-src-to-address-list protocol=udp dst-port={{.Stage1Port}} \
    address-list=mkpk-tt-stage1-{{.Key}} address-list-timeout={{$.StageTimeout}} \
    comment="mkpk-tt stage1 {{.Key}}"
add chain=input action=add-src-to-address-list protocol=udp dst-port={{.Stage2Port}} \
    src-address-list=mkpk-tt-stage1-{{.Key}} \
    address-list=mkpk-tt-stage2-{{.Key}} address-list-timeout={{$.StageTimeout}} \
    comment="mkpk-tt stage2 {{.Key}}"
{{end}}{{range .Clients}}add chain=input action=add-src-to-address-list protocol=udp dst-port={{.TokenPort}} \
    src-address-list=mkpk-tt-stage2-{{.ServiceKey}} content="mkpk-tt-token-not-initialized" \
    address-list=mkpk-tt-hit-now-{{.Key}} address-list-timeout={{$.TokenHitTimeout}} disabled=yes \
    comment="mkpk-tt token now {{.Key}}"
add chain=input action=add-src-to-address-list protocol=udp dst-port={{.TokenPort}} \
    src-address-list=mkpk-tt-stage2-{{.ServiceKey}} content="mkpk-tt-token-not-initialized" \
    address-list=mkpk-tt-hit-prev-{{.Key}} address-list-timeout={{$.TokenHitTimeout}} disabled=yes \
    comment="mkpk-tt token prev {{.Key}}"
{{end}}
/system script
add name="mkpk-tt-meta" policy=read source="# mkpk-version=1\n# mkpk-config-hash={{.MetaHash}}"
add name="mkpk-tt-apply-service" policy=read,write,test source={
{{range .Services}}{{if eq .TargetType "forward"}}    # forward target: dst-nat translates the destination, and a forward-chain
    # accept lets the translated (gated) packet reach the internal host.
    :if ([:len [/ip firewall nat find where comment={{.Comment}}]] = 0) do={
        /ip firewall nat add chain=dstnat action=dst-nat protocol={{.Protocol}} dst-port={{.Port}} \
            src-address-list={{.AllowedList}} to-addresses={{.ToAddress}} to-ports={{.ToPort}} \
            comment={{.Comment}}
    } else={
        /ip firewall nat set [/ip firewall nat find where comment={{.Comment}}] chain=dstnat action=dst-nat protocol={{.Protocol}} dst-port={{.Port}} \
            src-address-list={{.AllowedList}} to-addresses={{.ToAddress}} to-ports={{.ToPort}}
    }
    :if ([:len [/ip firewall filter find where comment={{.Comment}}]] = 0) do={
        :local mkpkDrop [/ip firewall filter find where chain=forward action=drop]
        :if ([:len $mkpkDrop] > 0) do={
            /ip firewall filter add chain=forward action=accept protocol={{.Protocol}} dst-address={{.ToAddress}} dst-port={{.ToPort}} \
                src-address-list={{.AllowedList}} comment={{.Comment}} place-before=[:pick $mkpkDrop 0]
        } else={
            /ip firewall filter add chain=forward action=accept protocol={{.Protocol}} dst-address={{.ToAddress}} dst-port={{.ToPort}} \
                src-address-list={{.AllowedList}} comment={{.Comment}}
        }
    } else={
        /ip firewall filter set [/ip firewall filter find where comment={{.Comment}}] chain=forward action=accept protocol={{.Protocol}} dst-address={{.ToAddress}} dst-port={{.ToPort}} \
            src-address-list={{.AllowedList}}
    }
    :log info ("mkpk-tt target forward applied comment=" . {{.Comment}})
{{else}}    # local target: accept in the input chain for gated sources reaching the router.
    :if ([:len [/ip firewall filter find where comment={{.Comment}}]] = 0) do={
        :local mkpkDrop [/ip firewall filter find where chain=input action=drop]
        :if ([:len $mkpkDrop] > 0) do={
            /ip firewall filter add chain=input action=accept protocol={{.Protocol}} dst-port={{.Port}} \
                src-address-list={{.AllowedList}} comment={{.Comment}} place-before=[:pick $mkpkDrop 0]
        } else={
            /ip firewall filter add chain=input action=accept protocol={{.Protocol}} dst-port={{.Port}} \
                src-address-list={{.AllowedList}} comment={{.Comment}}
        }
    } else={
        /ip firewall filter set [/ip firewall filter find where comment={{.Comment}}] chain=input action=accept protocol={{.Protocol}} dst-port={{.Port}} \
            src-address-list={{.AllowedList}}
    }
    :log info ("mkpk-tt target local applied comment=" . {{.Comment}})
{{end}}{{end}}}

add name="mkpk-tt-notify" policy=read,write,test source={
    # Router-level notification config, baked in at render time so it survives
    # reboot without relying on globals. The poller sets only the event context.
    :local nWebhook {{.NotifyWebhook}}
    :local nUrl {{.NotifyURL}}
    :local nTelegram {{.NotifyTelegram}}
    :local nBotToken {{.NotifyBotToken}}
    :local nChatId {{.NotifyChatID}}
    :local nThreadId {{.NotifyThreadID}}
    :local nEmail {{.NotifyEmail}}
    :local nEmailTo {{.NotifyEmailTo}}
    :local nEmailFrom {{.NotifyEmailFrom}}
    :local nEmailServer {{.NotifyEmailServer}}
    :local nEmailPort {{.NotifyEmailPort}}
    :local nEmailTls {{.NotifyEmailTLS}}
    :local nEmailUser {{.NotifyEmailUser}}
    :local nEmailPassword {{.NotifyEmailPassword}}

    :global mkpkTtNotifyRouter
    :global mkpkTtNotifyService
    :global mkpkTtNotifyClientId
    :global mkpkTtNotifySrc
    :global mkpkTtNotifyList
    :global mkpkTtNotifyTtl
    :global mkpkTtNotifyBucket
    :global mkpkTtNotifyTime

    # Channels are independent — every enabled one fires.
    :if (($nTelegram = true) && ([:len $nBotToken] > 0) && ([:len $nChatId] > 0)) do={
        # Human-readable, plain text (no parse_mode → nothing to escape):
        #   🔓 KZ-D2A: socks5 open for test2
        #   from 95.25.177.50 · 55m
        :local text ("\F0\9F\94\93 " . $mkpkTtNotifyRouter . ": " . $mkpkTtNotifyService . " open for " . $mkpkTtNotifyClientId . "\nfrom " . $mkpkTtNotifySrc . " \C2\B7 " . $mkpkTtNotifyTtl)
        :local tgBody ("{\"chat_id\":\"" . $nChatId . "\"")
        :if ([:len $nThreadId] > 0) do={
            :set tgBody ($tgBody . ",\"message_thread_id\":" . $nThreadId)
        }
        :set tgBody ($tgBody . ",\"text\":" . [:serialize $text to=json] . "}")
        :do {
            /tool fetch url=("https://api.telegram.org/bot" . $nBotToken . "/sendMessage") http-method=post http-header-field="Content-Type: application/json" http-data=$tgBody keep-result=no
        } on-error={
            :log warning ("mkpk-tt notify telegram failed src=" . $mkpkTtNotifySrc . " service=" . $mkpkTtNotifyService)
        }
    }

    :if (($nEmail = true) && ([:len $nEmailServer] > 0) && ([:len $nEmailTo] > 0)) do={
        :local subject ("mkpk allowed " . $mkpkTtNotifyService . " " . $mkpkTtNotifySrc)
        :local body ("router=" . $mkpkTtNotifyRouter . "\nservice=" . $mkpkTtNotifyService . "\nclient_id=" . $mkpkTtNotifyClientId . "\nsrc=" . $mkpkTtNotifySrc . "\nlist=" . $mkpkTtNotifyList . "\nttl=" . $mkpkTtNotifyTtl . "\nbucket=" . $mkpkTtNotifyBucket . "\ntime=" . $mkpkTtNotifyTime)
        :do {
            /tool e-mail send to=$nEmailTo from=$nEmailFrom server=$nEmailServer port=$nEmailPort tls=$nEmailTls user=$nEmailUser password=$nEmailPassword subject=$subject body=$body
        } on-error={
            :log warning ("mkpk-tt notify email failed src=" . $mkpkTtNotifySrc . " service=" . $mkpkTtNotifyService)
        }
    }

    :if (($nWebhook = true) && ([:len $nUrl] > 0)) do={
        :local payload [:serialize {"router"=$mkpkTtNotifyRouter; "service"=$mkpkTtNotifyService; "client_id"=$mkpkTtNotifyClientId; "src"=$mkpkTtNotifySrc; "list"=$mkpkTtNotifyList; "ttl"=$mkpkTtNotifyTtl; "mode"="udp-token"; "bucket"=$mkpkTtNotifyBucket; "time"=$mkpkTtNotifyTime} to=json]
        :do {
            /tool fetch url=$nUrl http-method=post http-header-field="Content-Type: application/json" http-data=$payload keep-result=no
        } on-error={
            :log warning ("mkpk-tt notify webhook failed src=" . $mkpkTtNotifySrc . " service=" . $mkpkTtNotifyService)
        }
    }
    :return 0
}

add name="mkpk-tt-poller" policy=read,write,test source={
    :global mkpkTtBucket
    :global mkpkTtClients

    :local nowBucket ([:timestamp] / {{.BucketSeconds}}s)

    # Build the client table once and cache it in a global; rebuilding this literal
    # every tick would dominate CPU. Cleared on (re)import and lost on reboot.
    :if ([:typeof $mkpkTtClients] = "nothing") do={
        :set mkpkTtClients {{.ClientsArray}}
    }
    :local clients $mkpkTtClients

    :local refreshTokens do={
        :local c $1
        :local nb $2
        :local pb ($nb - 1)
        :local key ($c->"key")
        :local nowRule [/ip firewall filter find where comment=("mkpk-tt token now " . $key)]
        :local prevRule [/ip firewall filter find where comment=("mkpk-tt token prev " . $key)]
        :if (([:len $nowRule] = 0) || ([:len $prevRule] = 0)) do={
            :log error ("mkpk-tt token rules missing for " . $key . "; fail-closed")
            :return 0
        }
        :local psk ($c->"psk")
        :local service ($c->"service")
        :local clientId ($c->"clientId")
        :local tokenPort ($c->"tokenPort")
        :local nowMsg ($psk . "|v1|" . $service . "|" . $clientId . "|" . $nb . "|" . $psk)
        :local prevMsg ($psk . "|v1|" . $service . "|" . $clientId . "|" . $pb . "|" . $psk)
        :local nowTok [:convert $nowMsg from=raw to=hex transform=sha512]
        :local prevTok [:convert $prevMsg from=raw to=hex transform=sha512]
        # Only set when the bucket actually rolled — RouterOS logs every rule
        # change, and the poller runs every 1s while the token changes every
        # bucket, so an unconditional set floods system,info.
        :if ([/ip firewall filter get $nowRule content] != $nowTok) do={
            /ip firewall filter set $nowRule content=$nowTok disabled=no dst-port=$tokenPort
        }
        :if ([/ip firewall filter get $prevRule content] != $prevTok) do={
            /ip firewall filter set $prevRule content=$prevTok disabled=no dst-port=$tokenPort
        }
        :return 0
    }

    :local processHits do={
        :local c $1
        :local nb $2
        :local pb ($nb - 1)
        :local key ($c->"key")
        :local nowHits [/ip firewall address-list find where list=("mkpk-tt-hit-now-" . $key)]
        :local prevHits [/ip firewall address-list find where list=("mkpk-tt-hit-prev-" . $key)]
        :local hitCount ([:len $nowHits] + [:len $prevHits])
        :if ($hitCount = 0) do={
            :return 0
        }
        :local service ($c->"service")
        :local clientId ($c->"clientId")
        :local allowedList ($c->"allowedList")
        :local allowedTimeout ($c->"allowedTimeout")
        :local usedTimeout ($c->"usedTimeout")
        :local selectedBucket $nb
        :local selectedHits $nowHits
        :if (([:len $nowHits] = 0) && ([:len $prevHits] > 0)) do={
            :set selectedBucket $pb
            :set selectedHits $prevHits
        }
        :local usedList ("mkpk-tt-used-" . $key . "-" . $selectedBucket)
        :if ([:len [/ip firewall address-list find where list=$usedList]] > 0) do={
            :log warning ("mkpk-tt replay ignored for " . $key . "; bucket already used; hits=" . $hitCount)
            /ip firewall address-list remove $nowHits
            /ip firewall address-list remove $prevHits
            :return 0
        }
        :if ($hitCount > 1) do={
            /ip firewall address-list add list=$usedList address=127.0.0.1 timeout=$usedTimeout comment=("bucket=" . $selectedBucket)
            :log warning ("mkpk-tt collision/replay suspicion for " . $key . "; hits=" . $hitCount . "; bucket burned")
            /ip firewall address-list remove $nowHits
            /ip firewall address-list remove $prevHits
            :return 0
        }
        :local hit [:pick $selectedHits 0]
        :local src [/ip firewall address-list get $hit address]
        /ip firewall address-list add list=$usedList address=127.0.0.1 timeout=$usedTimeout comment=("bucket=" . $selectedBucket)
        /ip firewall address-list remove [find where list=$allowedList address=$src]
        /ip firewall address-list add list=$allowedList address=$src timeout=$allowedTimeout \
            comment=("mkpk-tt client_id=" . $clientId . "; mode=udp-token; service=" . $service . "; bucket=" . $selectedBucket)
        :log info ("mkpk-tt allowed src=" . $src . " ttl=" . $allowedTimeout . " bucket=" . $selectedBucket . " client=" . $key)
        /ip firewall address-list remove $nowHits
        /ip firewall address-list remove $prevHits

{{if .NotifyActive}}        # Notification config is baked into mkpk-tt-notify; set only the event context.
        :global mkpkTtNotifyRouter [/system identity get name]
        :global mkpkTtNotifyService $service
        :global mkpkTtNotifyClientId $clientId
        :global mkpkTtNotifySrc $src
        :global mkpkTtNotifyList $allowedList
        :global mkpkTtNotifyTtl $allowedTimeout
        :global mkpkTtNotifyBucket $selectedBucket
        :global mkpkTtNotifyTime ([:timestamp] . "")
        /system script run mkpk-tt-notify
{{end}}        :return 0
    }

    :if ([:typeof $mkpkTtBucket] = "nothing") do={
        :set mkpkTtBucket 0
    }
    :if ($nowBucket != $mkpkTtBucket) do={
        :foreach c in=$clients do={ [$refreshTokens $c $nowBucket] }
        :set mkpkTtBucket $nowBucket
    }
    # Hot path: one regex find over all hit lists. Only iterate clients when a
    # token actually hit (the rare case), keeping idle per-tick cost ~constant.
    :if ([:len [/ip firewall address-list find where list~"^mkpk-tt-hit-"]] > 0) do={
        :foreach c in=$clients do={ [$processHits $c $nowBucket] }
    }
}

add name="mkpk-tt-startup" policy=read,write,test source={
    :global mkpkTtBucket
    :local invalidContent "mkpk-tt-token-not-initialized"
    :foreach rule in=[/ip firewall filter find where comment~"^mkpk-tt token "] do={
        /ip firewall filter set $rule content=$invalidContent disabled=yes
    }
    /ip firewall address-list remove [find where list~"^mkpk-tt-hit-"]
    :set mkpkTtBucket 0
    /system script run mkpk-tt-apply-service
    :log info "mkpk-tt startup guard applied"
    :return 0
}

/system scheduler
add name="mkpk-tt-startup" start-time=startup \
    on-event="/system script run mkpk-tt-startup" \
    policy=read,write,test comment="mkpk-tt fail-closed startup guard"
add name="mkpk-tt-poller" interval=1s start-time=startup \
    on-event="/system script run mkpk-tt-poller" \
    policy=read,write,test comment="mkpk-tt data-driven poller"
add name="mkpk-tt-install" interval=1s \
    on-event="/system scheduler remove [find where name=\"mkpk-tt-install\"]; /system script run mkpk-tt-startup" \
    policy=read,write,test comment="mkpk-tt one-shot post-import init"

/system script run mkpk-tt-apply-service
:log info "mkpk-tt installed"
`))

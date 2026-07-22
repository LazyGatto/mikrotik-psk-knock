# MikroTik PSK Knock time-token prototype.
#
# Purpose:
# - validate RouterOS-side PSK-derived time-token generation;
# - update firewall content rules for current and previous 30s buckets;
# - keep stage -> token-hit -> scheduler -> allowed behavior from the static prototype;
# - start fail-closed until scheduler has computed token contents.
#
# Demo profile:
#   service=demo-service
#   client_id=demo-client
#   psk=mkpk-prototype-psk
#
# Token formula:
#   sha512(psk + "|v1|" + service + "|" + client_id + "|" + bucket + "|" + psk)
#
# Client token helper:
#   bucket=$(($(date +%s) / 30))
#   printf 'mkpk-prototype-psk|v1|demo-service|demo-client|%s|mkpk-prototype-psk' "$bucket" | shasum -a 512 | awk '{print $1}'
#
# Test flow from a client:
#   token=$(printf 'mkpk-prototype-psk|v1|demo-service|demo-client|%s|mkpk-prototype-psk' "$(($(date +%s) / 30))" | shasum -a 512 | awk '{print $1}')
#   printf x | nc -u -w1 <router> 41001
#   printf x | nc -u -w1 <router> 41002
#   printf "$token" | nc -u -w1 <router> 41003
#
# Expected result:
#   /ip firewall address-list print where list=mkpk-tt-allowed
#
# Cleanup:
#   /system scheduler remove [find where name="mkpk-tt-poller"]
#   /system script remove [find where name="mkpk-tt-poller"]
#   /ip firewall filter remove [find where comment~"^mkpk-tt "]
#   /ip firewall address-list remove [find where list~"^mkpk-tt-"]

/system scheduler remove [find where name="mkpk-tt-poller"]
/system script remove [find where name="mkpk-tt-poller"]
/ip firewall filter remove [find where comment~"^mkpk-tt "]
/ip firewall address-list remove [find where list~"^mkpk-tt-"]

/ip firewall filter
add chain=input action=add-src-to-address-list protocol=udp dst-port=41001 \
    address-list=mkpk-tt-stage1 address-list-timeout=5s \
    comment="mkpk-tt stage1"
add chain=input action=add-src-to-address-list protocol=udp dst-port=41002 \
    src-address-list=mkpk-tt-stage1 \
    address-list=mkpk-tt-stage2 address-list-timeout=5s \
    comment="mkpk-tt stage2"
add chain=input action=add-src-to-address-list protocol=udp dst-port=41003 \
    src-address-list=mkpk-tt-stage2 content="mkpk-tt-token-not-initialized" \
    address-list=mkpk-tt-token-hit-now address-list-timeout=2s disabled=yes \
    comment="mkpk-tt token now"
add chain=input action=add-src-to-address-list protocol=udp dst-port=41003 \
    src-address-list=mkpk-tt-stage2 content="mkpk-tt-token-not-initialized" \
    address-list=mkpk-tt-token-hit-prev address-list-timeout=2s disabled=yes \
    comment="mkpk-tt token prev"

/system script
add name="mkpk-tt-poller" policy=read,write,test source={
    :local service "demo-service"
    :local clientId "demo-client"
    :local psk "mkpk-prototype-psk"
    :local tokenPort 41003
    :local allowedList "mkpk-tt-allowed"
    :local allowedTimeout "3m"
    :local usedTimeout "35s"
    :local nowBucket ([:timestamp] / 30s)
    :local prevBucket ($nowBucket - 1)

    :local nowMsg ($psk . "|v1|" . $service . "|" . $clientId . "|" . $nowBucket . "|" . $psk)
    :local prevMsg ($psk . "|v1|" . $service . "|" . $clientId . "|" . $prevBucket . "|" . $psk)
    :local nowToken [:convert $nowMsg from=raw to=hex transform=sha512]
    :local prevToken [:convert $prevMsg from=raw to=hex transform=sha512]

    :local nowRule [/ip firewall filter find where comment="mkpk-tt token now"]
    :local prevRule [/ip firewall filter find where comment="mkpk-tt token prev"]

    :if (([:len $nowRule] = 0) || ([:len $prevRule] = 0)) do={
        :log error "mkpk-tt token rules missing; fail-closed"
        :return 0
    }

    :if ([/ip firewall filter get $nowRule content] != $nowToken) do={
        /ip firewall filter set $nowRule content=$nowToken disabled=no dst-port=$tokenPort
    }
    :if ([/ip firewall filter get $prevRule content] != $prevToken) do={
        /ip firewall filter set $prevRule content=$prevToken disabled=no dst-port=$tokenPort
    }

    :local nowHits [/ip firewall address-list find where list="mkpk-tt-token-hit-now"]
    :local prevHits [/ip firewall address-list find where list="mkpk-tt-token-hit-prev"]
    :local hitCount ([:len $nowHits] + [:len $prevHits])

    :if ($hitCount = 0) do={
        :return 0
    }

    :local selectedBucket $nowBucket
    :local selectedHits $nowHits

    :if (([:len $nowHits] = 0) && ([:len $prevHits] > 0)) do={
        :set selectedBucket $prevBucket
        :set selectedHits $prevHits
    }

    :local usedList ("mkpk-tt-used-" . $selectedBucket)
    :local used [/ip firewall address-list find where list=$usedList]

    :if ([:len $used] > 0) do={
        :log warning ("mkpk-tt replay ignored; bucket already used; hits=" . $hitCount)
        /ip firewall address-list remove $nowHits
        /ip firewall address-list remove $prevHits
        :return 0
    }

    :if ($hitCount > 1) do={
        /ip firewall address-list add list=$usedList address=127.0.0.1 timeout=$usedTimeout comment=("bucket=" . $selectedBucket)
        :log warning ("mkpk-tt collision/replay suspicion; hits=" . $hitCount . "; bucket burned")
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
    :log info ("mkpk-tt allowed src=" . $src . " ttl=" . $allowedTimeout . " bucket=" . $selectedBucket)
    /ip firewall address-list remove $nowHits
    /ip firewall address-list remove $prevHits
    :return 0
}

/system scheduler
add name="mkpk-tt-poller" interval=1s start-time=startup \
    on-event="/system script run mkpk-tt-poller" \
    policy=read,write,test comment="mkpk-tt update tokens and poll hits"

:log info "mkpk-tt installed"

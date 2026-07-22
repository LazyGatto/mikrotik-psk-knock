# MikroTik PSK Knock end-to-end prototype.
#
# Purpose:
# - validate staged UDP knock -> token-hit address-list -> scheduler -> allowed list;
# - keep all objects clearly namespaced with mkpk-proto;
# - use a static test token payload before adding PSK/time-bucket generation.
#
# Test flow from a client:
#   printf x | nc -u -w1 <router> 41001
#   printf x | nc -u -w1 <router> 41002
#   printf mkpk-proto-token-v1 | nc -u -w1 <router> 41003
#
# Expected result:
#   /ip firewall address-list print where list=mkpk-proto-allowed
#
# Cleanup:
#   /system scheduler remove [find where name="mkpk-proto-poller"]
#   /system script remove [find where name="mkpk-proto-poller"]
#   /ip firewall filter remove [find where comment~"^mkpk-proto "]
#   /ip firewall address-list remove [find where list~"^mkpk-proto-"]

/system scheduler remove [find where name="mkpk-proto-poller"]
/system script remove [find where name="mkpk-proto-poller"]
/ip firewall filter remove [find where comment~"^mkpk-proto "]
/ip firewall address-list remove [find where list~"^mkpk-proto-"]

/ip firewall filter
add chain=input action=add-src-to-address-list protocol=udp dst-port=41001 \
    address-list=mkpk-proto-stage1 address-list-timeout=5s \
    comment="mkpk-proto stage1"
add chain=input action=add-src-to-address-list protocol=udp dst-port=41002 \
    src-address-list=mkpk-proto-stage1 \
    address-list=mkpk-proto-stage2 address-list-timeout=5s \
    comment="mkpk-proto stage2"
add chain=input action=add-src-to-address-list protocol=udp dst-port=41003 \
    src-address-list=mkpk-proto-stage2 content="mkpk-proto-token-v1" \
    address-list=mkpk-proto-token-hit address-list-timeout=2s \
    comment="mkpk-proto token"

/system script
add name="mkpk-proto-poller" policy=read,write,test source={
    :local tokenHitList "mkpk-proto-token-hit"
    :local allowedList "mkpk-proto-allowed"
    :local allowedTimeout "3m"
    :local usedTimeout "35s"
    :local bucket ([:timestamp] / 30s)
    :local usedList ("mkpk-proto-used-" . $bucket)
    :local bucketComment ("bucket=" . $bucket)
    :local hits [/ip firewall address-list find where list=$tokenHitList]
    :local hitCount [:len $hits]

    :if ($hitCount = 0) do={
        :return 0
    }

    :local used [/ip firewall address-list find where list=$usedList]

    :if ([:len $used] > 0) do={
        :log warning ("mkpk-proto replay ignored; bucket already used; hits=" . $hitCount)
        /ip firewall address-list remove $hits
        :return 0
    }

    :if ($hitCount > 1) do={
        /ip firewall address-list add list=$usedList address=127.0.0.1 timeout=$usedTimeout comment=$bucketComment
        :log warning ("mkpk-proto collision/replay suspicion; hits=" . $hitCount . "; bucket burned")
        /ip firewall address-list remove $hits
        :return 0
    }

    :local hit [:pick $hits 0]
    :local src [/ip firewall address-list get $hit address]

    /ip firewall address-list add list=$usedList address=127.0.0.1 timeout=$usedTimeout comment=$bucketComment
    /ip firewall address-list remove [find where list=$allowedList address=$src]
    /ip firewall address-list add list=$allowedList address=$src timeout=$allowedTimeout \
        comment=("mkpk-proto client_id=prototype; mode=udp-token; bucket=" . $bucket)
    :log info ("mkpk-proto allowed src=" . $src . " ttl=" . $allowedTimeout . " bucket=" . $bucket)
    /ip firewall address-list remove $hits
    :return 0
}

/system scheduler
add name="mkpk-proto-poller" interval=1s start-time=startup \
    on-event="/system script run mkpk-proto-poller" \
    policy=read,write,test comment="mkpk-proto poll token-hit"

:log info "mkpk-proto installed"

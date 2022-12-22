#!/bin/sh

# Finds dead functions successfully *most* of the time by comparing the
# exported symbols in a compiled binary to mangled `go doc` output.
# Very hacky, has some false positives I don't care enough to fix.
#
# Use with caution.

go tool nm ./fchan | grep fedichan | sed 's/.*fedichan\///;s/,.*//;s/\.func[0-9]//;/\*/d;/\.\./d' | sort | uniq > alive
for i in activitypub routes db config util; do
	go doc -short -all "$i" | grep func | sed "s/func //;/^(/ { s/([[:alpha:]]* //;s/) /./};s/(.*//;s/^/$i./"
done | sort > all

printf '!! Output not guaranteed to be correct. Ensure it is actually not used with ripgrep.\n\n' >&2

while read -r line; do
	grep -q "$line" alive || printf '%s\n' "$line"
done < all

rm alive all

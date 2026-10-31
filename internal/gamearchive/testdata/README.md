# Test archives

Small archives made with the real tools, since nothing in Go writes 7z or RAR. Every "game" in them
is random bytes with a made-up name, and every password is `secret`.

They were made on Ubuntu with 7-Zip 26.00 and RAR 7.20 like this, where each folder holds what its
name says:

```
7z a -mx=9 cart.7z .                         # Game (USA).sfc, readme.txt
rar a -m5 cart.rar .
rar a -m5 -s cart-solid.rar .
7z a -ms=on -mx=5 disc-solid.7z .            # Game.cue, two tracks, readme.txt
7z a -ms=off -mx=5 disc-notsolid.7z .
rar a -s disc-solid.rar .
7z a disc-nested.7z .                        # the same under inner/
7z a discs.7z .                              # two discs, a .cue and .bin for each
7z a -psecret locked-content.7z .            # names readable, contents locked
7z a -psecret -mhe=on locked-names.7z .      # names locked too
7z a -psecret -tzip locked.zip .
rar a -psecret locked-content.rar .
rar a -hpsecret locked-names.rar .
rar a -v100k -m0 split.rar .                 # split.part1.rar and on
7z a -snl link.7z .                          # holds a symbolic link
rar a -ol link.rar .
```

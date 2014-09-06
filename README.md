Paclan is a tool to share archlinux packages between machines on a LAN.
It is similar in functionalitis to
[Pacserve](http://xyne.archlinux.ca/projects/pacserve/), but aims at
simplicity.

Manual Installation
===================

1. Install the go toolchain (see http://golang.org/doc/install)
2. Build:

    ```
    $ cd /path/to/paclan
    $ go build
    ````

3. Copy the files in relevant places:

    ```
    $ cp paclan /usr/bin
    $ cp paclan.conf /etc/pacman.d/
    $ cp paclan.service /usr/lib/systemd/system/
    ```

4. Add the relevant `include` line in the pacman config
   (`/etc/pacman.conf`) for each repo where you wish to share packages
   on the LAN:

   ```
   Include = /etc/pacman.d/paclan.conf
   ```

Automatic Installation
======================

[A PKGBUILD is
provided](https://github.com/rakoo/PKGBUILDS/tree/master/paclan). Use standard makepkg procedures (see
https://wiki.archlinux.org/index.php/Makepkg) or install it directly
from the AUR.

Running
=======

Paclan isn't expected to be run manually, a systemd service file is
provided. For more details, see
https://wiki.archlinux.org/index.php/Systemd

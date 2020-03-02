Paclan is a tool to share archlinux packages between machines on a LAN.
It is similar in functionalitis to
[Pacserve](http://xyne.archlinux.ca/projects/pacserve/), but aims at
simplicity.

Official Fork
=============
Thank you to [rakoo](https://github.com/rakoo/paclan) for starting the paclan project.

As of March 2020, this is the officially maintained fork of paclan.


Manual Installation
===================

1. Install the go toolchain (see http://golang.org/doc/install)
2. Build:

    ```
    $ cd /path/to/paclan
    $ go build
    ```

3. Copy the files in relevant places:

    ```
    $ cp paclan /usr/bin
    $ cp paclan.conf /etc/pacman.d/
    $ cp mirrorlist.paclan /etc/pacman.d/
    $ cp paclan.service /usr/lib/systemd/system/
    ```

4. Add the relevant `include` line in the pacman config
   (`/etc/pacman.conf`) for each repo where you wish to share packages
   on the LAN:

   ```
   Include = /etc/pacman.d/mirrorlist.paclan
   ```

Automatic Installation
======================

Paclan is available to install directly from the [AUR](https://aur.archlinux.org/packages/paclan)

Running
=======

Paclan isn't expected to be run manually, a systemd service file is
provided. For more details, see [Systemd Wiki](https://wiki.archlinux.org/index.php/Systemd)

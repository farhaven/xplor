xplor, a tree-style (file) explorer for (plan9port) Acme
========================================================

* [screenshot, regular](https://user-images.githubusercontent.com/505/55195521-111fce80-51ad-11e9-9725-58ceae7c785d.png)
* [screenshot, monospaced](https://user-images.githubusercontent.com/505/55195505-02391c00-51ad-11e9-9293-7b58a37a49d7.png)

Xplor is written for [Acme, the Plan 9 text editing environment][acme].
I use Acme from [Plan 9 from User Space][plan9port].
To learn about Acme, [Russ Cox’s Tour of Acme][tour] is a great place to start.

[acme]: http://acme.cat-v.org
[plan9port]: https://9fans.github.io/plan9port/
[tour]: https://research.swtch.com/acme


Usage
-----

Button 3 (right click) on a directory to open or close it.
Button 3 on a file to plumb it, e.g. to open text or source files.
Button 2 (middle click) on any entry to prints its path in the Errors window.

`Win` and `Xplor` open a new [win][] or xplor window.
Button 2 to open those windows for the current xplor directory,
2-1 chord to open the selected directory instead.

`Get` reloads the current window.

`All` toggles whether xplor displays hidden entries.

`Up` opens parent of the current directory in the same window.

[win]: https://9fans.github.io/plan9port/man/man1/acme.html


Launch
------

	xplor

to open the current working directory, or

	xplor /path/to/directory

to open a specific directory.


Installation
------------

Xplor is `go get`able:

	go get -u git.sr.ht/~mkhl/xplor

Enjoy!

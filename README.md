## BufferKing

Cache it your way!
Record your computers audio output to an audio library.


### About

BufferKing listens for 'Play/Pause/Next/Previous' notifications from any music player using DBus. When it detects that a new track is being played it will start recording it.

### Setup virtual audio source

It is possible to create a temporary virtual audio source that a music app's audio can be redirected into.

1. Create sink:
    ```bash
    pactl load-module module-null-sink sink_name=music_sink
    ```
2. Forward sink audio to default audio output:
    ```bash
    pactl load-module module-loopback source=music_sink.monitor
    ```
3. List modules:
    ```bash
    pactl list modules short
    ```
4. To unload a listed module:
    ```bash
    pactl unload-module 536870916
    ```

### Example usage
To get started, a path to the root of the library you wish to record to is required as the first argument to BufferKing.
The audio source from which to record from is also needed; however BufferKing will present you with the available options if you don't provide an audio source.
```bash
$ BufferKing ~/Music
0) alsa_output.pci-0000_00_1f.3.analog-stereo.monitor
1) alsa_output.pci-0000_01_00.1.hdmi-stereo.monitor
Record from which source:
```

### Current Limitations

BufferKing doesn't handle pausing very well (at all) right now.
If a track is being recorded and is paused or skipped it is discarded by default.
This behavior can be changed using the `-P, --keep-paused` and `-S, --keep-skipped` flags if you wish to keep partial recordings to stitch together later or whatever.


### Contributing

Contributions are very welcome :)

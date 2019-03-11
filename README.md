# handover2019okwuibe

WebRTC demo which captures video of an UE in an LTE network using WebRTC, which is then passed onto a MEC server for post-processing, and finally returned back as a video stream to the client.

We expect the post-processing software to accept ivf containerized VP8 codec, and return an RTP stream on port 1234.

With FFMPEG (and applying a grayscale postprocessing) this can be achieved with the following command:

`ffmpeg -i - -vf format=gray -c:v libvpx -b:v 1M -f rtp udp://0.0.0.0:1234`

As such, because the software supports Unix pipes, the whole process is expect to be run as so:

1. `go build && handover2019okwuibe | ffmpeg -i - -vf format=gray -c:v libvpx -b:v 1M -f rtp udp://0.0.0.0:1234`
2. Opening the client file found in the `/static/demo.html` using your web browser
3. Allow video-access
4. A local video feed with the post-processed one will appear side-by-side

In this demo, the WebRTC tokens are passed over HTTP, which, as such, expects you to know the address of the MEC server beforehand. One could thus ask why is WebRTC used, which is reasoned by demonstrating the interoperability of the client software (any UE capable of handling a modern browser can use the MEC service), in addition to demonstrating modern protocols with the capability of port hole punching, and achieving supposed better performance by leveraging UDP instead of regular TCP connections regular to HTTP services.

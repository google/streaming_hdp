# iterations: 3
# urlfile:"/home/vaspol/Desktop/experiments/config_files/debug_urls"

iterations: 25
# iterations: 1
urlfile:"/home/vaspol/Desktop/experiments/config_files/shdp_urls"

configs:{
  label:"Streaming HD Previews"
  connectivity:"3G_EM"
  device:"Motorola G - Chrome"
  location:"MotoG"
  command_line:"--ignore-certificate-errors --proxy-pac-url=http://35.188.237.140:8449/streaming_hdp_route.pac --disable-default-apps --disable-extensions"
  timeline: true
  trace: true
  skip_video: false
}

configs:{
  label:"HD Previews"
  connectivity:"3G_EM"
  device:"Motorola G - Chrome"
  location:"MotoG"
  command_line:"--ignore-certificate-errors --proxy-pac-url=http://35.188.237.140:8449/hdpreviews_route.pac --disable-default-apps --disable-extensions"
  timeline: true
  trace: true
  skip_video: false
}

configs:{
  label:"No Proxy"
  connectivity:"3G_EM"
  device:"Motorola G - Chrome"
  location:"MotoG"
  command_line:"--ignore-certificate-errors --disable-default-apps --disable-extensions"
  timeline: true
  trace: true
  skip_video: false
}

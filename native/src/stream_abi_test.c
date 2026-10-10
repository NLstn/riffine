/* Compile and execute as C: opaque API must not require C++ headers/types. */
#include "riffine_stream.h"
int main(void) {
  riffine_stream_config config={1,1,48000,64,1,0};
  riffine_stream *stream=0;
  char error[256];
  riffine_stream_limits limits;
  if(riffine_stream_create(&config,&stream,error,sizeof(error))) return 1;
  int result=riffine_stream_get_limits(stream,&limits,error,sizeof(error));
  riffine_stream_destroy(stream);
  return result || limits.max_input_frames!=64;
}

/* Compile and execute as C: opaque API must not require C++ headers/types. */
#include "riffine_stream.h"
#include "riffine_preview.h"
#include "riffine_selection.h"
int main(void) {
  riffine_stream_config config={1,1,48000,64,1,0};
  riffine_stream *stream=0;
  char error[256];
  riffine_stream_limits limits;
  if(riffine_stream_create(&config,&stream,error,sizeof(error))) return 1;
  int result=riffine_stream_get_limits(stream,&limits,error,sizeof(error));
  riffine_stream_destroy(stream);
  riffine_preview_source *source=0;
  if(riffine_preview_open(0,0,1,1,0,&source,error,sizeof(error))!=1 || source) return 1;
  riffine_selection *selection=0;
  float input=.2f,output=0;
  uint32_t count=0,needs_input=0;
  if(riffine_selection_create(1,48000,1,2,0,&selection,error,sizeof(error))) return 1;
  int selection_result=riffine_selection_push(selection,&input,1,error,sizeof(error));
  selection_result|=riffine_selection_read(selection,&output,1,&count,&needs_input,error,sizeof(error));
  riffine_selection_destroy(selection);
  return result || limits.max_input_frames!=64 || selection_result || count!=1 || needs_input;
}

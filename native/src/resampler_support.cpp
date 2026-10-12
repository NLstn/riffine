/* Browser processing uses the same pinned miniaudio resampler as native,
 * excluding platform/device/thread subsystems from the WASM worker. */
#define MA_NO_DEVICE_IO
#define MA_NO_ENCODING
#define MA_NO_ENGINE
#define MA_NO_RESOURCE_MANAGER
#define MA_NO_NODE_GRAPH
#define MA_NO_THREADING
#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"

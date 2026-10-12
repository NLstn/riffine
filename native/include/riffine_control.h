#ifndef RIFFINE_CONTROL_H
#define RIFFINE_CONTROL_H
#include <stdint.h>
/* Internal off-realtime helper protocol. Retains a borrowed source until EOF,
 * explicit close or process exit. Fixed LE request/response frames; metadata
 * only, never PCM or presentation acknowledgements. */
int riffine_scheduled_control(uint64_t source_handle);
#endif

// Offline in-process probe of the existing shared C++ DSP ABI. Generated media
// only; this does not test streaming, mobile output devices or WASM DSP.
#include "riffine_audio.h"
#include <cstdio>
int main(int argc, char **argv) {
 if (argc != 3) return 2;
 char error[1024] = {};
 int result = riffine_render(argv[1], argv[2], 12000, 36000, 0.5, -2, error, sizeof(error));
 if (result) std::fprintf(stderr, "%s\n", error);
 return result;
}

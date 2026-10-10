/* Link this same caller against native, iOS-simulator or Android Go libraries.
 * The host only owns file input and ABI memory, never project/practice policy. */
#include <stdio.h>
#include <stdlib.h>
#include "libhost-spike.h"
int main(int argc, char **argv) {
 if (argc != 2) { fputs("fixture path required\n", stderr); return 2; }
 FILE *f = fopen(argv[1], "rb");
 if (!f) { perror("fixture"); return 2; }
 char *input = calloc(1048578, 1);
 if (!input) { fclose(f); return 2; }
 size_t n = fread(input, 1, 1048577, f);
 int failed = ferror(f); fclose(f);
 if (failed || n > 1048576) { free(input); return 2; }
 char *result = RiffineRunFixture(input);
 free(input);
 if (!result) return 2;
 puts(result); RiffineFree(result);
 return 0;
}

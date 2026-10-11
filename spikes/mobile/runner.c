/* Link this same caller against native, iOS-simulator or Android Go libraries.
 * The host only owns file input and ABI memory, never project/practice policy. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include "libhost-spike.h"
static double milliseconds(void) {
 struct timespec ts;
 if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0) return -1;
 return ts.tv_sec * 1000.0 + ts.tv_nsec / 1000000.0;
}
static int compare_double(const void *a, const void *b) {
 double left = *(const double *)a, right = *(const double *)b;
 return (left > right) - (left < right);
}
int main(int argc, char **argv) {
 int iterations = 1;
 if (argc == 3) {
  char *end = NULL;
  long count = strtol(argv[2], &end, 10);
  if (!end || *end || count < 1 || count > 256) return 2;
  iterations = (int)count;
 }
 if (argc != 2 && argc != 3) { fputs("fixture path required\n", stderr); return 2; }
 FILE *f = fopen(argv[1], "rb");
 if (!f) { perror("fixture"); return 2; }
 char *input = calloc(1048578, 1);
 if (!input) { fclose(f); return 2; }
 size_t n = fread(input, 1, 1048577, f);
 int failed = ferror(f); fclose(f);
 if (failed || n > 1048576 || memchr(input, 0, n)) { free(input); return 2; }
 char *first = NULL;
 double runs[256];
 for (int i = 0; i < iterations; ++i) {
  double started = milliseconds();
  char *result = RiffineRunFixture(input);
  double finished = milliseconds();
  if (!result || started < 0 || finished < started) { free(input); free(first); if (result) RiffineFree(result); return 2; }
  runs[i] = finished - started;
  if (!first) {
   first = malloc(strlen(result) + 1);
   if (!first) { RiffineFree(result); free(input); return 2; }
   strcpy(first, result);
  } else if (strcmp(first, result) != 0) {
   fputs("nondeterministic repeated ABI report\n", stderr);
   RiffineFree(result); free(input); free(first); return 2;
  }
  RiffineFree(result);
 }
 free(input);
 puts(first); free(first);
 double first_ms = runs[0];
 qsort(runs, (size_t)iterations, sizeof(double), compare_double);
 double median = iterations % 2 ? runs[iterations/2] : (runs[iterations/2-1] + runs[iterations/2])/2;
 fprintf(stderr, "{\"iterations\":%d,\"firstFixtureMs\":%.3f,\"medianFixtureMs\":%.3f,\"minFixtureMs\":%.3f,\"maxFixtureMs\":%.3f}\n", iterations, first_ms, median, runs[0], runs[iterations-1]);
 return 0;
}

cmake_minimum_required(VERSION 3.24)
project(riffine_browser_processing LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
include(FetchContent)
# Same immutable revisions as native and the processing conformance probe.
FetchContent_Declare(signalsmith-linear
 GIT_REPOSITORY https://github.com/Signalsmith-Audio/linear.git
 GIT_TAG de55e6a50ffcf6f8f43f649692d94691c7025151 GIT_SUBMODULES "")
FetchContent_Declare(signalsmith-stretch
 GIT_REPOSITORY https://github.com/Signalsmith-Audio/signalsmith-stretch.git
 GIT_TAG a670068d9aeb64913331d5cc29337b19a457a7df GIT_SUBMODULES "")
FetchContent_MakeAvailable(signalsmith-linear signalsmith-stretch)
add_executable(riffine-browser-dsp ../src/stream.cpp ../src/selection.cpp)
target_include_directories(riffine-browser-dsp PRIVATE ../include)
target_link_libraries(riffine-browser-dsp PRIVATE signalsmith-stretch)
if(NOT EMSCRIPTEN)
 message(FATAL_ERROR "Use the activated Emscripten SDK for the browser processing target")
endif()
set_target_properties(riffine-browser-dsp PROPERTIES SUFFIX ".mjs")
target_compile_options(riffine-browser-dsp PRIVATE -fexceptions)
target_link_options(riffine-browser-dsp PRIVATE --no-entry -fexceptions
 "-sDISABLE_EXCEPTION_CATCHING=0" "-sMODULARIZE=1" "-sEXPORT_ES6=1"
 "-sENVIRONMENT=web,worker,node" "-sWASM_BIGINT=1"
 "-sEXPORTED_FUNCTIONS=[\"_malloc\",\"_free\",\"_riffine_selection_create\",\"_riffine_selection_info_get\",\"_riffine_selection_push\",\"_riffine_selection_read\",\"_riffine_selection_destroy\"]"
 "-sEXPORTED_RUNTIME_METHODS=[\"HEAPU8\",\"HEAPF32\"]"
 "-sINITIAL_MEMORY=33554432" "-sALLOW_MEMORY_GROWTH=0" "-sEXIT_RUNTIME=0")

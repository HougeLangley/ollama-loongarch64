#ifndef __APPLE__  // TODO - maybe consider nvidia support on intel macs?

#include <string.h>
#include "gpu_info_nvcuda.h"

void nvcuda_init(char *nvcuda_lib_path, nvcuda_init_resp_t *resp) {
  CUresult ret;
  resp->err = NULL;
  resp->num_devices = 0;
  const int buflen = 256;
  char buf[buflen + 1];
  int i;

  struct lookup {
    char *s;
    void **p;
  } l[] = {
   
      {"cuInit", (void *)&resp->ch.cuInit},
      {"cuDriverGetVersion", (void *)&resp->ch.cuDriverGetVersion},
      {"cuDeviceGetCount", (void *)&resp->ch.cuDeviceGetCount},
      {"cuDeviceGet", (void *)&resp->ch.cuDeviceGet},
      {"cuDeviceGetAttribute", (void *)&resp->ch.cuDeviceGetAttribute},
      {"cuDeviceGetUuid", (void *)&resp->ch.cuDeviceGetUuid},
      {"cuDeviceGetName", (void *)&resp->ch.cuDeviceGetName},
      {"cuCtxCreate_v3", (void *)&resp->ch.cuCtxCreate_v3},
      {"cuMemGetInfo_v2", (void *)&resp->ch.cuMemGetInfo_v2},
      {"cuCtxDestroy", (void *)&resp->ch.cuCtxDestroy},
      {NULL, NULL},
  };

  resp->ch.handle = LOAD_LIBRARY(nvcuda_lib_path, RTLD_LAZY);
  if (!resp->ch.handle) {
    char *msg = LOAD_ERR();
    LOG(resp->ch.verbose, "library %s load err: %s\n", nvcuda_lib_path, msg);
    snprintf(buf, buflen,
            "Unable to load %s library to query for Nvidia GPUs: %s",
            nvcuda_lib_path, msg);
    free(msg);
    resp->err = strdup(buf);
    return;
  }

  for (i = 0; l[i].s != NULL; i++) {
    *l[i].p = LOAD_SYMBOL(resp->ch.handle, l[i].s);
    if (!*l[i].p) {
      char *msg = LOAD_ERR();
      LOG(resp->ch.verbose, "dlerr: %s\n", msg);
      UNLOAD_LIBRARY(resp->ch.handle);
      resp->ch.handle = NULL;
      snprintf(buf, buflen, "symbol lookup for %s failed: %s", l[i].s,
              msg);
      free(msg);
      resp->err = strdup(buf);
      return;
    }
  }

  ret = (*resp->ch.cuInit)(0);
  if (ret != CUDA_SUCCESS) {
    LOG(resp->ch.verbose, "cuInit err: %d\n", ret);
    UNLOAD_LIBRARY(resp->ch.handle);
    resp->ch.handle = NULL;
    if (ret == CUDA_ERROR_INSUFFICIENT_DRIVER) {
      resp->err = strdup("your nvidia driver is too old or missing.  If you have a CUDA GPU please upgrade to run ollama");
      return;
    }
    snprintf(buf, buflen, "nvcuda init failure: %d", ret);
    resp->err = strdup(buf);
    return;
  }

  int version = 0;
  resp->ch.driver_major = 0;
  resp->ch.driver_minor = 0;

  // Report driver version if we're in verbose mode, ignore errors
  ret = (*resp->ch.cuDriverGetVersion)(&version);
  if (ret != CUDA_SUCCESS) {
    LOG(resp->ch.verbose, "cuDriverGetVersion failed: %d\n", ret);
  } else {
    resp->ch.driver_major = version / 1000;
    resp->ch.driver_minor = (version - (resp->ch.driver_major * 1000)) / 10;
    LOG(resp->ch.verbose, "CUDA driver version: %d.%d\n", resp->ch.driver_major, resp->ch.driver_minor);
  }

  ret = (*resp->ch.cuDeviceGetCount)(&resp->num_devices);
  if (ret != CUDA_SUCCESS) {
    LOG(resp->ch.verbose, "cuDeviceGetCount err: %d\n", ret);
    UNLOAD_LIBRARY(resp->ch.handle);
    resp->ch.handle = NULL;
    snprintf(buf, buflen, "unable to get device count: %d", ret);
    resp->err = strdup(buf);
    return;
  }
}

const int buflen = 256;
void nvcuda_check_vram(nvcuda_handle_t h, int i, mem_info_t *resp) {
  resp->err = NULL;
  nvcudaMemory_t memInfo = {0,0};
  CUresult ret;
  CUdevice device = -1;
  CUcontext ctx = NULL;
  char buf[buflen + 1];
  CUuuid uuid = {0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0};

  if (h.handle == NULL) {
    resp->err = strdup("nvcuda handle isn't initialized");
    return;
  }

  ret = (*h.cuDeviceGet)(&device, i);
  if (ret != CUDA_SUCCESS) {
    snprintf(buf, buflen, "nvcuda device failed to initialize");
    resp->err = strdup(buf);
    return;
  }

  int major = 0;
  int minor = 0;
  ret = (*h.cuDeviceGetAttribute)(&major, CU_DEVICE_ATTRIBUTE_COMPUTE_CAPABILITY_MAJOR, device);
  if (ret != CUDA_SUCCESS) {
    LOG(h.verbose, "[%d] device major lookup failure: %d\n", i, ret);
  } else {
    ret = (*h.cuDeviceGetAttribute)(&minor, CU_DEVICE_ATTRIBUTE_COMPUTE_CAPABILITY_MINOR, device);
    if (ret != CUDA_SUCCESS) {
      LOG(h.verbose, "[%d] device minor lookup failure: %d\n", i, ret);
    } else {
      resp->minor = minor;  
      resp->major = major;  
    }
  }

  ret = (*h.cuDeviceGetUuid)(&uuid, device);
  if (ret != CUDA_SUCCESS) {
    LOG(h.verbose, "[%d] device uuid lookup failure: %d\n", i, ret);
    snprintf(&resp->gpu_id[0], GPU_ID_LEN, "%d", i);
  } else {
    // GPU-d110a105-ac29-1d54-7b49-9c90440f215b
    snprintf(&resp->gpu_id[0], GPU_ID_LEN,
        "GPU-%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
        uuid.bytes[0],
        uuid.bytes[1],
        uuid.bytes[2],
        uuid.bytes[3],
        uuid.bytes[4],
        uuid.bytes[5],
        uuid.bytes[6],
        uuid.bytes[7],
        uuid.bytes[8],
        uuid.bytes[9],
        uuid.bytes[10],
        uuid.bytes[11],
        uuid.bytes[12],
        uuid.bytes[13],
        uuid.bytes[14],
        uuid.bytes[15]
      );
  }

  ret = (*h.cuDeviceGetName)(&resp->gpu_name[0], GPU_NAME_LEN, device);
  if (ret != CUDA_SUCCESS) {
    LOG(h.verbose, "[%d] device name lookup failure: %d\n", i, ret);
    resp->gpu_name[0] = '\0';
  }

  // To get memory we have to set (and release) a context
  ret = (*h.cuCtxCreate_v3)(&ctx, NULL, 0, 0, device);
  if (ret != CUDA_SUCCESS) {
    snprintf(buf, buflen, "nvcuda failed to get primary device context %d", ret);
    resp->err = strdup(buf);
    return;
  }

  ret = (*h.cuMemGetInfo_v2)(&memInfo.free, &memInfo.total);
  if (ret != CUDA_SUCCESS) {
    snprintf(buf, buflen, "nvcuda device memory info lookup failure %d", ret);
    resp->err = strdup(buf);
    // Best effort on failure...
    (*h.cuCtxDestroy)(ctx);
    return;
  }

  resp->total = memInfo.total;
  resp->free = memInfo.free;

  LOG(h.verbose, "[%s] CUDA totalMem %lu mb\n", resp->gpu_id, resp->total / 1024 / 1024);
  LOG(h.verbose, "[%s] CUDA freeMem %lu mb\n", resp->gpu_id, resp->free / 1024 / 1024);
  LOG(h.verbose, "[%s] Compute Capability %d.%d\n", resp->gpu_id, resp->major, resp->minor);

  

  ret = (*h.cuCtxDestroy)(ctx);
  if (ret != CUDA_SUCCESS) {
    LOG(1, "nvcuda failed to release primary device context %d", ret);
  }
}

void nvcuda_release(nvcuda_handle_t h) {
  LOG(h.verbose, "releasing nvcuda library\n");
  UNLOAD_LIBRARY(h.handle);
  // TODO and other context release logic?
  h.handle = NULL;
}

#endif  // __APPLE__
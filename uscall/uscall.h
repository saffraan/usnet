#ifndef __USCALL_H__
#define __USCALL_H__

#include <stdint.h>
#include <sys/types.h>
//  set/clear non-blocking i/o
int ff_ioctl_non_bio(int fd, int on);
int sys_ioctl_non_bio(int fd, int on);

int sys_set_reuse_port(int fd);

typedef struct slice{
	char* ptr;
	uint32_t len;
	uint32_t cap;
}slice;

slice* slice_alloc(uint32_t cap);
int slice_alloc_1(slice* s, uint32_t cap);

void slice_free(slice* s);
void slice_free_1(slice* s);
void slice_clean(slice *s);

ssize_t ff_read_slice(int fd, slice* output);
ssize_t ff_write_slice(int fd, slice*  input);

ssize_t ff_read_cslice(int fd, slice* output);
ssize_t ff_write_cslice(int fd, slice*  input);

ssize_t sys_read_cslice(int fd, slice* output);
ssize_t sys_write_cslice(int fd, slice*  input);

int slice_child(const slice* parent, slice *child,  uint32_t pos,  uint32_t len);

#endif
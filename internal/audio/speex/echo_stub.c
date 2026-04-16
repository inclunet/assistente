#include "config.h"
#include "arch.h"
#include "speex/speex_echo.h"

void speex_echo_get_residual(SpeexEchoState *st, spx_word32_t *residual_echo, int len) {
	(void)st;
	if (residual_echo == 0 || len <= 0) {
		return;
	}
	for (int i = 0; i < len; ++i) {
		residual_echo[i] = 0;
	}
}

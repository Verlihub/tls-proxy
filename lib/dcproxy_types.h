#ifndef DCPROXY_TYPES_H
#define DCPROXY_TYPES_H

#include <stdbool.h>

typedef struct DCProxyConfig {
	const char * HubAddr;
	const char * HubNetwork;
	const char * Hosts; // comma separated
	const char * Cert;
	const char * Key;
	const char * CertOrg;
	const char * CertHost;
	const char * PProf;
	const char * Metrics;
	bool LogErrors;
	int  Wait; // ms
	int  Buffer; // KB
	bool NoSendIP;
} DCProxyConfig;

#endif // DCPROXY_TYPES_H
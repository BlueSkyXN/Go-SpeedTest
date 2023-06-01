def plan0():
    return {
        'url': {
            'base_url': 'https://speed.cloudflare.com/__down?bytes=500000000',
            'disable_ssl_verification': 'true',
            'ssl_domain': 'speed.cloudflare.com',
            'host_domain': 'speed.cloudflare.com',
            'lock_ip': '104.27.200.69',
            'lock_port': '443',
        },
        'Speed': {
            'connections': '8',
            'test_duration': '60',
        }
    }

def plan1():
    return {
        'url': {
            'base_url': 'https://speed.cloudflare.com/__down?bytes=400000000',
            'disable_ssl_verification': 'true',
            'ssl_domain': 'speed.cloudflare.com',
            'host_domain': 'speed.cloudflare.com',
            'lock_ip': '104.27.200.70',
            'lock_port': '443',
        },
        'Speed': {
            'connections': '10',
            'test_duration': '70',
        }
    }

# Add more plans here...

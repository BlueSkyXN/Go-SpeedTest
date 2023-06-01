# 放入所有的计划
plans = [
    {
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
    },
    {
        'url': {
            'base_url': 'https://download.parallels.com/desktop/v17/17.1.1-51537/ParallelsDesktop-17.1.1-51537.dmg',
            'ssl_domain': 'download.parallels.com',
            'host_domain': 'download.parallels.com',
        },
        'Speed': {
        }
    },
    {
        'url': {
            'base_url': 'https://cloudflare.cdn.openbsd.org/pub/OpenBSD/7.3/alpha/install73.iso',
            'ssl_domain': 'cloudflare.cdn.openbsd.org',
            'host_domain': 'cloudflare.cdn.openbsd.org',
        },
        'Speed': {
        }
    },
    {
        'url': {
            'base_url': 'https://cdn.openbsd.org/pub/OpenBSD/7.3/amd64/install73.iso',
            'ssl_domain': 'cdn.openbsd.org',
            'host_domain': 'cdn.openbsd.org',
        },
        'Speed': {
        }
    },
    {
        'url': {
            'base_url': 'https://repo.steampowered.com/arch/valveaur/mesa-aco-git-debug-20.1.0_devel.20200404.ffc7574ff73-7-x86_64.pkg.tar.xz',
            'ssl_domain': 'repo.steampowered.com',
            'host_domain': 'repo.steampowered.com',
        },
        'Speed': {
        }
    },
    {
        'url': {
            'base_url': 'https://cdn.cloudflare.steamstatic.com/steam/apps/256843155/movie_max.mp4',
            'ssl_domain': 'cdn.cloudflare.steamstatic.com',
            'host_domain': 'cdn.cloudflare.steamstatic.com',
        },
        'Speed': {
        }
    },
    {
        'url': {
            'base_url': 'https://speedtest.poorhub.pro/cf.7z',
            'ssl_domain': 'speedtest.poorhub.pro',
            'host_domain': 'speedtest.poorhub.pro',
        },
        'Speed': {
        }
    },
    # ... 更多的计划 ...
]

# 通过索引获取计划
def get_plan(plan_num):
    # 判断传入的 plan_num 是否在 plans 的索引范围内
    if isinstance(plan_num, int) and 0 <= plan_num < len(plans):
        return plans[plan_num]
    else:
        return plans[0]  # 如果 plan_num 不符合要求，返回第0条计划（plan0）

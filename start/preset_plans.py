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
            'base_url': 'https://speed.cloudflare.com/__down?bytes=400000000',
            'ssl_domain': 'speed.cloudflare.com',
            'host_domain': 'speed.cloudflare.com',
            'lock_ip': '104.27.200.70',
        },
        'Speed': {
            'connections': '10',
            'test_duration': '70',
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

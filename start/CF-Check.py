import requests
from concurrent.futures import ThreadPoolExecutor, as_completed
import openpyxl
import threading

# 目标URL的基础部分
base_url = "http://{ip}/cdn-cgi/trace"
host_header = "www.nodeseek.com"  # 目标域名

# 并发数量
concurrent_requests = 5000  # 调整并发数量以适应实际情况

# 生成需要测试的IP列表，检查全部的X.X.X.7
def generate_ip_list():
    ip_list = []
    for i in range(104, 172):  # 避免生成 0.0.0.7 这种无效 IP
        for j in range(128):
            for k in range(1):
                ip_list.append(f"{i}.{j}.{k}.7")
    return ip_list

# 进行网络请求
def check_connectivity(ip):
    headers = {
        "Host": host_header,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
    }
    target_url = base_url.format(ip=ip)
    try:
        response = requests.get(target_url, timeout=5, headers=headers, verify=False)
        if response.status_code == 200:
            response_text = response.text
            if f'h={host_header}' in response_text and 'ip=120.40.56.233' in response_text:
                lines = response_text.split('\n')
                ip_value = colo_value = sliver_value = None
                for line in lines:
                    if line.startswith('ip='):
                        ip_value = line.split('=')[1]
                    elif line.startswith('colo='):
                        colo_value = line.split('=')[1]
                    elif line.startswith('sliver='):
                        sliver_value = line.split('=')[1]
                if ip_value and colo_value and sliver_value:
                    print(f"成功的请求目标IP: {ip}")
                    return ip, ip_value, colo_value, sliver_value
        else:
            print(f"响应状态码: {response.status_code}")
    except requests.RequestException as e:
        print(f"请求异常: {e}")
    return None

# 保存结果到xlsx文件
def save_to_excel(results, file_path="connectivity_results.xlsx"):
    workbook = openpyxl.Workbook()
    sheet = workbook.active
    sheet.append(['目标IP', 'IP', 'Colo', 'Sliver'])
    for result in results:
        sheet.append(result)
    workbook.save(file_path)

# 主函数
def main():
    ip_list = generate_ip_list()
    results = []
    total_ips = len(ip_list)
    current_ip_count = 0
    progress_lock = threading.Lock()
    
    def increment_progress():
        nonlocal current_ip_count
        with progress_lock:
            current_ip_count += 1
            print(f"当前进度: {current_ip_count}/{total_ips}")

    with ThreadPoolExecutor(max_workers=concurrent_requests) as executor:
        future_to_ip = {executor.submit(check_connectivity, ip): ip for ip in ip_list}
        for future in as_completed(future_to_ip):
            increment_progress()
            result = future.result()
            if result:
                results.append(result)

    save_to_excel(results)

if __name__ == "__main__":
    main()

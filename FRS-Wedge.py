import socket
import time
import subprocess
import datetime
import configparser
import platform
import logging

# Configure logging
logging.basicConfig(filename='broadcast.log', level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Load configuration
config = configparser.ConfigParser()
config.read('config.ini')
ip_address = config['DEFAULT']['IP_Address']
callsign = config['DEFAULT']['Callsign']
nickname = config['DEFAULT']['Nickname']
version = config['DEFAULT']['Version']
serial = config['DEFAULT']['Serial']
model = config['DEFAULT']['Model']
radio_license = config['DEFAULT']['Radio_License']

# Determine the ping command based on the OS
ping_option = '-n' if platform.system() == 'Windows' else '-c'

# Print the warning message on the screen
print("""
WARNING: USE AT YOUR OWN RISK

This script is provided "as is," without warranty of any kind, express or implied, 
including but not limited to the warranties of merchantability, fitness for a 
particular purpose, and noninfringement. In no event shall the authors or copyright 
holders be liable for any claim, damages, or other liability, whether in an action 
of contract, tort, or otherwise, arising from, out of, or in connection with the script 
or the use or other dealings in the script.

By using this script, you acknowledge and agree that you understand this warning, 
and any use of the script is entirely at your own risk. Any damage caused by the 
deployment or use of this script is the sole responsibility of the user, and the 
authors or distributors of this script cannot be held liable for any adverse 
consequences arising therefrom.
""")

# Pause the program and ask for user consent
user_input = input("Type YES to continue and acknowledge all risks: ")

if user_input != "YES":
    print("User did not acknowledge the risks. Exiting.")
    logging.info("User did not acknowledge the risks. Exiting.")
    exit()

# Print user settings
print("\nUser Settings\n")
print(f"Radio IP Address: {ip_address}")
print(f"Call Sign: {callsign}")
print(f"Nickname: {nickname}")
print(f"Version: {version}")
print(f"Serial Number: {serial}")
print(f"Model: {model}")
print(f"Radio License: {radio_license}")
print("\n")

# Define the broadcast address and the UDP port number
broadcast_address = '255.255.255.255'
port = 4992

# Create and configure the UDP socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)

last_status = None

try:
    while True:
        # Ping the IP address
        try:
            response = subprocess.run(['ping', ping_option, '1', ip_address], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            current_status = 'successful' if response.returncode == 0 else 'failed'
            
            if current_status == 'successful':
                message_text = f'discovery_protocol_version=3.0.0.2 model={model} serial={serial} version={version} nickname={nickname} callsign={callsign} ip={ip_address} port=4992 status=Available inuse_ip= inuse_host= max_licensed_version=v3 radio_license_id={radio_license} requires_additional_license=0 fpc_mac= wan_connected=1 licensed_clients=2 available_clients=2 max_panadapters=4 available_panadapters=4 max_slices=4 available_slices=4 gui_client_ips= gui_client_hosts= gui_client_programs= gui_client_stations= gui_client_handles= \x00\x00\x00'
                message = b'8T\x00\x8a\x00\x00\x08\x00\x00\x00\x1c-SL\xff\xfff!Hx\x00\x00\x00\x00\x00\x00\x00\x00' + message_text.encode('utf-8')
                sock.sendto(message, (broadcast_address, port))
                print("Message sent!")
                if last_status != current_status:
                    logging.info("Ping successful and message sent.")
                    last_status = current_status
                time.sleep(11)
            else:
                current_time = datetime.datetime.now().strftime("%H:%M:%S")
                print(f"{current_time} - Ping failed, will retry in 10 seconds...")
                if last_status != current_status:
                    logging.warning(f"{current_time} - Ping status changed to failed.")
                    last_status = current_status
                time.sleep(10)

        except Exception as e:
            print(f"Error: {e}")
            if last_status != 'exception':
                logging.error(f"Error during ping or UDP broadcast: {e}")
                last_status = 'exception'
finally:
    sock.close()
    logging.info("Socket closed and program terminated.")
    print("Socket closed and program terminated.")


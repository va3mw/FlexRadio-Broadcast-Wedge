"""
WARNING: USE AT YOUR OWN RISK

This script is provided "as is," without warranty of any kind, express or implied, 
including but not limited to the warranties of merchantability, fitness for a particular purpose, 
and noninfringement. In no event shall the authors or copyright holders be liable for any claim, 
damages, or other liability, whether in an action of contract, tort, or otherwise, arising from, 
out of, or in connection with the script or the use or other dealings in the script.

By using this script, you acknowledge and agree that you understand this warning, 
and any use of the script is entirely at your own risk. Any damage caused by the 
deployment or use of this script is the sole responsibility of the user, and the authors or 
distributors of this script cannot be held liable for any adverse consequences arising therefrom.

This script is not supported in anyway by FlexRadio Inc. 

Here are the specific dependencies:

Python Version:
Python 3.x: Ensure Python 3 is installed. Python 2.x is not supported. You can download Python 3 from the official Python website.

Standard Libraries: No installation of external packages is required as the script uses only built-in libraries, which include:

socket: For creating network connections and sending data over these connections.
subprocess: For running the ping command and capturing its output.
time: For handling delays in the script execution.
datetime: For generating timestamps.

System Requirements

Operating System: The script is designed for Windows. Commands and behaviors might differ on other operating systems.

To run this on Linux may require some minor edits including change the -n switch to -c.

Network Permissions: 

Ensure that the script has the necessary permissions to execute network-related 
commands, such as sending UDP packets and executing ping operations. Firewall and antivirus settings 
may need to be adjusted to allow these operations.

Usage:

Users must change ALL the variables to match the target radio.

After starting, broadcast messages on 255.255.255.255 should be visable to the Radio choose on any client
and the operator can connect to the radio directly over a VPN or another Subnet.  Users will need to ensure 
that UDP traffic can be passed on the VPN tunnel.

After the client is connected to the radio, this script can be terminated if desiired, however, there is no
risk in leaving it running 100% of the time.

Written by VA3MW with the help of ChatGPT4 - May 2024

"""

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

# Construct the dynamic message
message_text = f'discovery_protocol_version=3.0.0.2 model={model} serial={serial} version={version} nickname={nickname} callsign={callsign} ip={ip_address} port=4992 status=Available inuse_ip= inuse_host= max_licensed_version=v3 radio_license_id={radio_license} requires_additional_license=0 fpc_mac= wan_connected=1 licensed_clients=2 available_clients=2 max_panadapters=4 available_panadapters=4 max_slices=4 available_slices=4 gui_client_ips= gui_client_hosts= gui_client_programs= gui_client_stations= gui_client_handles= \x00\x00\x00'
message = b'8T\x00\x8a\x00\x00\x08\x00\x00\x00\x1c-SL\xff\xfff!Hx\x00\x00\x00\x00\x00\x00\x00\x00' + message_text.encode('utf-8')

# Create a UDP socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)

try:
    while True:
        # Ping the IP address
        try:
            response = subprocess.run(['ping', ping_option, '1', ip_address], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            if response.returncode == 0:
                sock.sendto(message, (broadcast_address, port))
                logging.info("Message sent successfully.")
                print("Message sent!")
                time.sleep(11)  # Regular interval
            else:
                current_time = datetime.datetime.now().strftime("%H:%M:%S")
                logging.warning(f"{current_time} - Ping failed, will retry in 10 seconds...")
                print(f"{current_time} - Ping failed, will retry in 10 seconds...")
                time.sleep(10)  # Retry interval
        except Exception as e:
            logging.error(f"Error during ping or UDP broadcast: {e}")
            print(f"Error: {e}")
finally:
    sock.close()
    logging.info("Socket closed and program terminated.")

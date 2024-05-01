import subprocess
import platform
import time
import socket

# Function to perform a ping
def ping(ip_address):
    # Determine the command based on the operating system
    param = '-n' if platform.system().lower() == 'windows' else '-c'
    command = ['ping', param, '1', ip_address]
    try:
        response = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        return response.returncode == 0
    except Exception as e:
        print(f"Failed to execute ping. Error: {e}")
        return False

# Main script
ip_address = '192.168.110.76'  # target radio IP address
previous_ping_status = None

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
    exit()

# Define the broadcast address and the UDP port number
broadcast_address = '255.255.255.255'
port = 4992

# Construct the dynamic message incorporating the variables as a complete string first
message_text = f'discovery_protocol_version=3.0.0.2 model=FLEX-6600 serial=0519-0079-6600-2411 version=3.8.2.29415 nickname=Flex6600 callsign=VA3MW ip={ip_address} port=4992 status=Available inuse_ip= inuse_host= max_licensed_version=v3 radio_license_id=00-1C-2D-05-0E-AA requires_additional_license=0 fpc_mac= wan_connected=1 licensed_clients=2 available_clients=2 max_panadapters=4 available_panadapters=4 max_slices=4 available_slices=4 gui_client_ips= gui_client_hosts= gui_client_programs= gui_client_stations= gui_client_handles= \x00\x00\x00'
message = b'8T\x00\x8a\x00\x00\x08\x00\x00\x00\x1c-SL\xff\xfff!Hx\x00\x00\x00\x00\x00\x00\x00\x00' + message_text.encode('utf-8')

# Create a UDP socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
# Set the option to enable broadcast
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)

try:
    while True:  # Loop to continuously send messages and monitor ping
        # Send the UDP message
        sock.sendto(message, (broadcast_address, port))
        print("UDP message sent!")

        # Check ping response
        success = ping(ip_address)
        current_ping_status = 'successful' if success else 'not successful'

        # Report the initial status or any change in status
        if previous_ping_status is None:
            print(f"Initial ping was {current_ping_status}.")
        elif previous_ping_status != current_ping_status:
            print(f"Change in ping status detected: It was {previous_ping_status}, now it is {current_ping_status}.")

        previous_ping_status = current_ping_status
        time.sleep(11)  # Wait for 11 seconds before the next loop

finally:
    # Ensure the socket is closed properly
    sock.close()
    print("Socket closed and program terminated.")
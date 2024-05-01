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

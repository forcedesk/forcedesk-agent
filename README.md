# SDAgent

SDAgent is the Agent for the SchoolDesk Helpdesk system.

It collects and sends data to the SchoolDesk server such as

- Network Probe Checks (Ping, ICMP, etc.)
Where a SchoolDesk Administrator has configured monitoring for their local network, the checks are performed by the agent and sent to the cloud-hosted tenant.

- User Directory Objects (LDAP/AD).
The contents of the local Active Directory users are sent to the SchoolDesk server. The user records there are then synchronized against the data. This helps import new staff and students into the portal.
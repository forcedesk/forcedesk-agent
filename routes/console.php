<?php

use Illuminate\Support\Facades\Schedule;

/*
|--------------------------------------------------------------------------
| Console Routes
|--------------------------------------------------------------------------
|
| This file is where you may define all of your Closure based console
| commands. Each Closure is bound to a command instance allowing a
| simple approach to interacting with each command's IO methods.
|
*/

/* Process Monitoring Payloads */
Schedule::command('agent:monitoring-service')->everyMinute()->withoutOverlapping()->runInBackground();

/* Process Device Manager Backup Payloads */
Schedule::command('agent:devicemanager-service')->everyMinute()->withoutOverlapping()->runInBackground();

/* Check for Kiosk Password Reset Requests */
Schedule::command('agent:kiosk-service')->everyFiveSeconds()->withoutOverlapping()->runInBackground();

/* Synchronize Papercut Data and send to tenant */
Schedule::command('agent:papercut-service')->everyThirtyMinutes()->withoutOverlapping()->runInBackground();

/* Synchronize Local Users */
Schedule::command('agent:usersync-service')->everyFiveMinutes()->withoutOverlapping()->runInBackground();

/* Synchronize EMC Users */
Schedule::command('agent:edustar-service')->daily()->withoutOverlapping()->runInBackground();

/* Synchronize CRT Accounts Users */
Schedule::command('agent:crtaccount-service')->daily()->withoutOverlapping()->runInBackground();

/* Send Agent Heartbeat */
Schedule::command('agent:heartbeat')->everyFiveMinutes()->withoutOverlapping()->runInBackground();

/* Process Command Queues */
Schedule::command('agent:process-command-queue')->everyMinute()->withoutOverlapping()->runInBackground();

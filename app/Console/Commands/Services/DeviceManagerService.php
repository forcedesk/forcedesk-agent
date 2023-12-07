<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use Illuminate\Console\Command;

class DeviceManagerService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:devicemanager-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Backs up managed devices and sends the data to the SchoolDesk Tenant.';

    public function handle()
    {

        // Find all the switches to backup.
        $devices = DeviceManagerDevices::all();
        $backupcount = 0;
        $errors = 0;
        $backuptype = $this->option('type');

        $this->comment('['.\Carbon\Carbon::now().'] Backups Started');

        // Create a unique batch ID for the jobs.
        $batchid = Str::random(8);

        foreach ($devices as $device) {

            // If backups are disabled for the device, do not include it.
            if ($device->backups_enabled == false) {
                $log = new DeviceManagerLogs;
                $log->device_id = $device->id;
                $log->logdata = '['.$batchid.'] Backups are disabled for Device: '.$device->name.'. Skipping.';
                $this->info('['.\Carbon\Carbon::now().'] Backups are disabled for Device: '.$device->name.'. Skipping.');
                $log->save();

                return false;
            }

            // Set Backup as Pending.
            $device->backup_pending = true;
            $device->save();

            $log = new DeviceManagerLogs;
            $log->device_id = $device->id;
            $log->logdata = '['.$batchid.'] Performing Backup on Device: '.$device->name;
            $this->info('['.\Carbon\Carbon::now().'] Performing Backup on Device: '.$device->name);
            $log->save();

            // If the device has backups enabled and has a credential attached, perform the backup.
            if ($device->credential) {

                // Store the credential as a temporary file.
                $passwordfile = tmpfile();

                $passwordfileuri = stream_get_meta_data($passwordfile)['uri'];

                fwrite($passwordfile, $device->credential->password);

                // The following is tailored for Cisco IOS/IOS-XE devices using password auth. May move to public key auth in future.
                if ($device->type == 'cisco') {
                    $process = Process::run('sshpass -f '.$passwordfileuri.' ssh -p '.$device->port.' -o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1 '.$device->credential->username.'@'.$device->hostname." 'more system:running-config'");
                } elseif ($device->type == 'mikrotik') {
                    $process = Process::run('sshpass -f '.$passwordfileuri.' ssh -p '.$device->port.' -o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1 '.$device->credential->username.'@'.$device->hostname." 'export show-sensitive verbose'");
                }

                if ($process->successful()) {

                    if (strlen($process->output()) >= 10) {

                        // For Cisco IOS/IOS-XE devices strip dynamic data before the version number.\
                        $output = $process->output();

                        if ($device->type == 'cisco') {
                            $garbagestring = strstr($output, '!');
                            $configdata = strstr($garbagestring, 'version');
                        } elseif ($device->type == 'mikrotik') {
                            $configdata = strstr($output, '/interface');
                        }

                        // See if there is an existing backup and check whether it matches the current backup.
                        $latest_backup = DeviceManagerBackups::latest()->where('device_id', $device->id)->first();

                        if (strlen($configdata) >= 10) {
                            if ($backuptype == 'monthly') {

                                $backup = new DeviceManagerBackups;
                                $backup->device_id = $device->id;

                                $backup->data = $configdata;
                                $backup->uuid = Str::uuid();
                                $backup->batch = $batchid;
                                $backup->size = strlen($backup->data);
                                $backup->save();

                                $log = new DeviceManagerLogs;
                                $log->device_id = $device->id;
                                $log->logdata = '['.$batchid.'] Monthly Full Backup for Device: '.$device->name.' was successful';
                                $this->info('['.\Carbon\Carbon::now().'] Monthly Full Backup for Device: '.$device->name.' was successful');
                                $log->save();
                                $backupcount++;

                                $device->backup_pending = false;
                                $device->lastbackedup_at = now();
                                $device->save();

                            } elseif ($latest_backup && hash('sha512', $latest_backup->data) == hash('sha512', $configdata)) {

                                $log = new DeviceManagerLogs;
                                $log->device_id = $device->id;
                                $log->logdata = '['.$batchid.'] Skipped Backing Up Device: '.$device->name.'. No changes were detected.';
                                $this->info('['.$batchid.'] Skipped Backing Up Device: '.$device->name.'. No changes were detected.');
                                $log->save();

                                $device->backup_pending = false;
                                $device->lastbackedup_at = now();
                                $device->save();

                            } else {
                                $backup = new DeviceManagerBackups;
                                $backup->device_id = $device->id;

                                $backup->data = $configdata;
                                $backup->uuid = Str::uuid();
                                $backup->batch = $batchid;
                                $backup->size = strlen($backup->data);
                                $backup->save();

                                $log = new DeviceManagerLogs;
                                $log->device_id = $device->id;
                                $log->logdata = '['.$batchid.'] Backup for Device: '.$device->name.' was successful';
                                $this->info('['.\Carbon\Carbon::now().'] Backup for Device: '.$device->name.' was successful');
                                $log->save();
                                $backupcount++;

                                $device->backup_pending = false;
                                $device->lastbackedup_at = now();
                                $device->save();
                            }

                        } else {
                            $log = new DeviceManagerLogs;
                            $log->device_id = $device->id;
                            $log->logdata = '['.$batchid.'] Backup for Device: '.$device->name.' was unsuccessful. No data was returned from the device. See Framework Logs';
                            $this->error('['.\Carbon\Carbon::now().'] Backup for Device: '.$device->name.' was unsuccessful. No data was returned from the device. See Framework Logs');
                            \Log::info('Failed Backup for ID: '.$device->id.'. Data: '.base64_encode($process->output()));
                            $log->save();
                            $errors++;

                            $device->backup_pending = false;
                            $device->save();
                        }

                    } else {
                        $log = new DeviceManagerLogs;
                        $log->device_id = $device->id;
                        $log->logdata = '['.$batchid.'] Backup for Device: '.$device->name.' failed. No data was returned from the device. See Framework Logs.';
                        $this->error('['.\Carbon\Carbon::now().'] Backup for Device: '.$device->name.' failed. No data was returned from the device. See Framework Logs.');
                        \Log::info('Failed Backup for ID: '.$device->id.'. Data: '.base64_encode($process->output()));
                        $errors++;
                        $log->save();

                        $device->backup_pending = false;
                        $device->save();
                    }

                } elseif ($process->failed()) {
                    $log = new DeviceManagerLogs;
                    $log->device_id = $device->id;
                    $log->logdata = '['.$batchid.'] Backup for Device: '.$device->name.' failed. Reason: '.$process->output();
                    $this->error('['.\Carbon\Carbon::now().'] Backup for Device: '.$device->name.' failed.');
                    $errors++;
                    $log->save();

                    $device->backup_pending = false;
                    $device->save();
                }

                // Overwrite the temporary file with garbage.
                fwrite($passwordfile, Str::random(4096));
                fclose($passwordfile);

            }
        }

        if ($errors > 0) {
            $this->error('['.\Carbon\Carbon::now().'] Backups completed at with '.$errors.' errors.');
        } else {
            $this->comment('['.\Carbon\Carbon::now().'] Backups completed with no errors.');
        }

    }
}

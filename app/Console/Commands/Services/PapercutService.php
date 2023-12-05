<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Jobs\PapercutAgent;
use App\Models\Students;
use App\Models\User;
use GuzzleHttp\Client;
use Illuminate\Console\Command;
use function Laravel\Prompts\progress;

class PapercutService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:papercut-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Retrieves and sends Papercut data to tenant.';

    public function handle(): bool
    {

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        PapercutAgent::dispatch();
        $this->info('Papercut Service job dispatched...');

        return true;
    }
}

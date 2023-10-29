<?php

namespace App\Commands;

use LaravelZero\Framework\Commands\Command;
use Pipetower\PhpSdk\Pipetower;
use function Termwind\render;

class ServersListCommand extends Command
{
    /** @var string */
    protected $signature = 'servers:list';

    /** @var string */
    protected $description = 'List all servers';

    /**
     * Execute the console command.
     */
    public function handle(Pipetower $pt): void
    {
        render(view('servers-list', ['servers' => $pt->servers->all()]));
    }
}

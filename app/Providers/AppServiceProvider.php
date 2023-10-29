<?php

namespace App\Providers;

use Illuminate\Support\ServiceProvider;
use Pipetower\PhpSdk\Pipetower;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        $this->app->singleton(Pipetower::class, fn() => new Pipetower(env('PIPETOWER_API_TOKEN', '')));
    }

    /**
     * Register any application services.
     */
    public function register(): void
    {
        //
    }
}
